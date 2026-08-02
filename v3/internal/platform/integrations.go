package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func (s *Service) ListIntegrationProbes(ctx context.Context, integration string, limit int) ([]IntegrationProbe, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,integration,target,status,latency_ms,detail,COALESCE(error_message,''),checked_by,checked_at FROM integration_probe_results WHERE ($1='' OR integration=$1) ORDER BY checked_at DESC LIMIT $2`, normalize(integration), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IntegrationProbe
	for rows.Next() {
		var item IntegrationProbe
		var raw []byte
		if err = rows.Scan(&item.ID, &item.Integration, &item.Target, &item.Status, &item.LatencyMS, &raw, &item.ErrorMessage, &item.CheckedBy, &item.CheckedAt); err != nil {
			return nil, err
		}
		item.Detail = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ProbeIntegration(ctx context.Context, integration string, instanceID *uuid.UUID, actor identity.Actor) (IntegrationProbe, error) {
	integration = normalize(integration)
	if !contains([]string{"emby", "tmdb", "moviepilot", "telegram"}, integration) {
		return IntegrationProbe{}, identity.ErrInvalid
	}
	started := time.Now()
	target := ""
	detail := map[string]any{}
	var probeErr error
	switch integration {
	case "emby":
		var instance EmbyInstance
		if instanceID != nil {
			instance, probeErr = s.GetInstance(ctx, *instanceID)
		} else {
			var items []EmbyInstance
			items, probeErr = s.ListInstances(ctx)
			if probeErr == nil {
				for _, candidate := range items {
					if candidate.Enabled {
						instance = candidate
						break
					}
				}
				if instance.ID == uuid.Nil {
					probeErr = identity.ErrNotFound
				}
			}
		}
		if probeErr == nil {
			target = safeTarget(instance.BaseURL)
			var token string
			token, probeErr = s.credentialSecret(ctx, instance.CredentialName)
			if probeErr == nil {
				var client *embyClient
				client, probeErr = newEmbyClient(instance, token)
				if probeErr == nil {
					var info embySystemInfo
					var latency time.Duration
					info, latency, probeErr = client.probe(ctx)
					detail = map[string]any{"instance_id": instance.ID, "server_id": info.ID, "server_name": info.ServerName, "version": info.Version, "remote_latency_ms": latency.Milliseconds()}
				}
			}
		}
	case "tmdb":
		base := strings.TrimRight(s.dynamicString(ctx, "tmdb.api_base_url", "https://api.themoviedb.org"), "/")
		target = safeTarget(base)
		var token string
		token, probeErr = s.credentialSecret(ctx, s.dynamicString(ctx, "tmdb.credential_name", "tmdb.api_token"))
		if probeErr == nil {
			var response map[string]any
			response, probeErr = probeJSON(ctx, base+"/3/configuration", map[string]string{"Authorization": bearerToken(token)})
			if probeErr == nil {
				_, hasImages := response["images"]
				detail = map[string]any{"configuration_received": true, "image_configuration_present": hasImages}
			}
		}
	case "moviepilot":
		base := strings.TrimRight(s.dynamicString(ctx, "moviepilot.api_base_url", "http://moviepilot:3000"), "/")
		target = safeTarget(base)
		var token string
		token, probeErr = s.credentialSecret(ctx, s.dynamicString(ctx, "moviepilot.credential_name", "moviepilot.api_token"))
		if probeErr == nil {
			path := normalizedUpstreamPath(s.dynamicString(ctx, "moviepilot.health_path", "/api/v1/system/setting"), "/api/v1/system/setting")
			var response map[string]any
			response, probeErr = probeJSON(ctx, base+path, map[string]string{"Authorization": bearerToken(token), "X-API-KEY": rawToken(token)})
			if probeErr == nil {
				detail = map[string]any{"response_received": true}
				for _, key := range []string{"success", "version"} {
					if value, ok := response[key]; ok {
						detail[key] = value
					}
				}
			}
		}
	case "telegram":
		base := strings.TrimRight(s.dynamicString(ctx, "telegram.api_base_url", "https://api.telegram.org"), "/")
		target = safeTarget(base)
		var token string
		token, probeErr = s.credentialSecret(ctx, "telegram.bot_token")
		if probeErr == nil {
			var response map[string]any
			response, probeErr = probeJSON(ctx, base+"/bot"+url.PathEscape(token)+"/getMe", nil)
			if result, ok := response["result"].(map[string]any); ok {
				detail = map[string]any{"id": result["id"], "username": result["username"], "first_name": result["first_name"]}
				if okValue, exists := response["ok"].(bool); exists && !okValue {
					probeErr = fmt.Errorf("Telegram rejected getMe")
				}
			}
		}
	}
	item := IntegrationProbe{ID: uuid.New(), Integration: integration, Target: target, Status: "healthy", LatencyMS: int(time.Since(started).Milliseconds()), Detail: detail, CheckedBy: actor.Label()}
	if probeErr != nil {
		item.Status = "unhealthy"
		item.ErrorMessage = truncateError(probeErr)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return IntegrationProbe{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO integration_probe_results(id,integration,target,status,latency_ms,detail,error_message,checked_by) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8) RETURNING checked_at`, item.ID, item.Integration, item.Target, item.Status, item.LatencyMS, jsonBytes(item.Detail), item.ErrorMessage, item.CheckedBy).Scan(&item.CheckedAt)
	if err == nil {
		err = audit(ctx, tx, actor, "integration.probe", "integration", integration, map[string]any{"status": item.Status, "latency_ms": item.LatencyMS, "target": item.Target, "instance_id": instanceID, "error": item.ErrorMessage})
	}
	if err != nil {
		return IntegrationProbe{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IntegrationProbe{}, err
	}
	return item, nil
}

func probeJSON(ctx context.Context, endpoint string, headers map[string]string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, sanitizeURLError(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}
	result := map[string]any{}
	if len(strings.TrimSpace(string(body))) == 0 {
		return result, nil
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return map[string]any{"http_status": response.StatusCode, "content_type": response.Header.Get("Content-Type")}, nil
	}
	return result, nil
}

func safeTarget(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "configured target"
	}
	return parsed.Scheme + "://" + parsed.Host
}
