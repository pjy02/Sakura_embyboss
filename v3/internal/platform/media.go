package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func (s *Service) dynamicString(ctx context.Context, key, fallback string) string {
	var value string
	if err := s.db.QueryRow(ctx, `SELECT value #>> '{}' FROM dynamic_settings WHERE key=$1`, key).Scan(&value); err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (s *Service) credentialSecret(ctx context.Context, name string) (string, error) {
	if s.vault == nil {
		return "", errors.New("credential vault is unavailable")
	}
	var ciphertext, nonce []byte
	var version int
	if err := s.db.QueryRow(ctx, `SELECT ciphertext,nonce,key_version FROM credentials WHERE name=$1`, name).Scan(&ciphertext, &nonce, &version); err != nil {
		return "", notFound(err)
	}
	plaintext, err := s.vault.Decrypt(ciphertext, nonce, version)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Service) SearchTMDB(ctx context.Context, query string, page int) ([]Media, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 1 || len(query) > 200 {
		return nil, identity.ErrInvalid
	}
	if page < 1 || page > 1000 {
		page = 1
	}
	credentialName := s.dynamicString(ctx, "tmdb.credential_name", "tmdb.api_token")
	token, err := s.credentialSecret(ctx, credentialName)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(s.dynamicString(ctx, "tmdb.api_base_url", "https://api.themoviedb.org"), "/")
	endpoint, err := url.Parse(base + "/3/search/multi")
	if err != nil {
		return nil, identity.ErrInvalid
	}
	values := endpoint.Query()
	values.Set("query", query)
	values.Set("page", fmt.Sprint(page))
	values.Set("language", s.dynamicString(ctx, "tmdb.language", "zh-CN"))
	values.Set("include_adult", "false")
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", bearerToken(token))
	request.Header.Set("Accept", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("TMDB search failed: %w", sanitizeURLError(err))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("TMDB returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	var body struct {
		Results []map[string]any `json:"results"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, maxEmbyResponse)).Decode(&body); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	ids := make([]uuid.UUID, 0, len(body.Results))
	for _, raw := range body.Results {
		mediaType := stringValue(raw["media_type"])
		if mediaType != "movie" && mediaType != "tv" {
			continue
		}
		externalID := numberValue(raw["id"])
		title := stringValue(raw["title"])
		originalTitle := stringValue(raw["original_title"])
		release := stringValue(raw["release_date"])
		if mediaType == "tv" {
			title = stringValue(raw["name"])
			originalTitle = stringValue(raw["original_name"])
			release = stringValue(raw["first_air_date"])
		}
		if externalID < 1 || strings.TrimSpace(title) == "" {
			continue
		}
		id := uuid.New()
		err = tx.QueryRow(ctx, `INSERT INTO media_catalog(id,external_id,media_type,title,original_title,overview,release_date,poster_path,backdrop_path,popularity,vote_average,metadata) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')::date,NULLIF($8,''),NULLIF($9,''),$10,$11,$12) ON CONFLICT(provider,external_id,media_type) DO UPDATE SET title=EXCLUDED.title,original_title=EXCLUDED.original_title,overview=EXCLUDED.overview,release_date=EXCLUDED.release_date,poster_path=EXCLUDED.poster_path,backdrop_path=EXCLUDED.backdrop_path,popularity=EXCLUDED.popularity,vote_average=EXCLUDED.vote_average,metadata=EXCLUDED.metadata,last_refreshed_at=NOW(),updated_at=NOW() RETURNING id`, id, externalID, mediaType, title, originalTitle, stringValue(raw["overview"]), release, stringValue(raw["poster_path"]), stringValue(raw["backdrop_path"]), floatValue(raw["popularity"]), floatValue(raw["vote_average"]), jsonBytes(raw)).Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	out := make([]Media, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetMedia(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func scanMedia(row rowScanner) (Media, error) {
	var item Media
	var raw []byte
	err := row.Scan(&item.ID, &item.Provider, &item.ExternalID, &item.MediaType, &item.Title, &item.OriginalTitle, &item.Overview, &item.ReleaseDate, &item.PosterPath, &item.BackdropPath, &item.Popularity, &item.VoteAverage, &raw, &item.LastRefreshed, &item.AvailableCount)
	item.Metadata = decodeJSON(raw)
	item.Available = item.AvailableCount > 0
	return item, notFound(err)
}

const mediaSelect = `SELECT m.id,m.provider,m.external_id,m.media_type,m.title,COALESCE(m.original_title,''),COALESCE(m.overview,''),m.release_date,COALESCE(m.poster_path,''),COALESCE(m.backdrop_path,''),m.popularity,m.vote_average,m.metadata,m.last_refreshed_at,COUNT(mm.id) FILTER(WHERE mm.status='matched') FROM media_catalog m LEFT JOIN media_matches mm ON mm.media_id=m.id`

func (s *Service) GetMedia(ctx context.Context, id uuid.UUID) (Media, error) {
	return scanMedia(s.db.QueryRow(ctx, mediaSelect+` WHERE m.id=$1 GROUP BY m.id`, id))
}

func (s *Service) ListMedia(ctx context.Context, query string, limit int) ([]Media, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, mediaSelect+` WHERE ($1='' OR LOWER(m.title) LIKE '%'||LOWER($1)||'%' OR LOWER(COALESCE(m.original_title,'')) LIKE '%'||LOWER($1)||'%') GROUP BY m.id ORDER BY m.popularity DESC,m.updated_at DESC LIMIT $2`, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		item, scanErr := scanMedia(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListMediaMatches(ctx context.Context, mediaID uuid.UUID) ([]MediaMatch, error) {
	rows, err := s.db.Query(ctx, `SELECT mm.id,mm.media_id,mm.instance_id,i.name,mm.status,COALESCE(mm.remote_item_id,''),COALESCE(mm.remote_title,''),COALESCE(mm.remote_item_type,''),mm.remote_snapshot,COALESCE(mm.last_error,''),mm.matched_at,mm.last_checked_at FROM media_matches mm JOIN emby_instances i ON i.id=mm.instance_id WHERE mm.media_id=$1 ORDER BY i.priority,i.name`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaMatch
	for rows.Next() {
		var item MediaMatch
		var raw []byte
		if err = rows.Scan(&item.ID, &item.MediaID, &item.InstanceID, &item.InstanceName, &item.Status, &item.RemoteItemID, &item.RemoteTitle, &item.RemoteItemType, &raw, &item.LastError, &item.MatchedAt, &item.LastCheckedAt); err != nil {
			return nil, err
		}
		item.RemoteSnapshot = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) EnqueueMediaMatches(ctx context.Context, mediaID uuid.UUID, key string, actor identity.Actor) ([]Task, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 120 {
		return nil, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var mediaExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_catalog WHERE id=$1)`, mediaID).Scan(&mediaExists); err != nil || !mediaExists {
		return nil, identity.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT id FROM emby_instances WHERE enabled ORDER BY priority,id`)
	if err != nil {
		return nil, err
	}
	var instanceIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		instanceIDs = append(instanceIDs, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		taskID := uuid.New()
		idempotency := "media-match:" + mediaID.String() + ":" + instanceID.String() + ":" + key
		var existing uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM platform_tasks WHERE idempotency_key=$1`, idempotency).Scan(&existing)
		if err == nil {
			ids = append(ids, existing)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		matchID := uuid.New()
		if _, err = tx.Exec(ctx, `INSERT INTO media_matches(id,media_id,instance_id,status) VALUES($1,$2,$3,'pending') ON CONFLICT(media_id,instance_id) DO UPDATE SET status='pending',last_error=NULL,updated_at=NOW()`, matchID, mediaID, instanceID); err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'media.match',$2,$3,8,$4)`, taskID, idempotency, jsonBytes(map[string]any{"instance_id": instanceID.String(), "media_id": mediaID.String()}), actor.Label())
		if err != nil {
			return nil, err
		}
		ids = append(ids, taskID)
	}
	if err = audit(ctx, tx, actor, "media.match.enqueue", "media", mediaID.String(), map[string]any{"instances": len(instanceIDs)}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetTask(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) CreateMediaRequest(ctx context.Context, accountID, mediaID uuid.UUID, note string, actor identity.Actor) (MediaRequest, error) {
	note = strings.TrimSpace(note)
	if len(note) > 1000 {
		return MediaRequest{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MediaRequest{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize requests for the same media so concurrent Web and Bot submissions
	// resolve to one canonical request instead of racing the partial unique index.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, mediaID.String()); err != nil {
		return MediaRequest{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_catalog WHERE id=$1)`, mediaID).Scan(&exists); err != nil || !exists {
		return MediaRequest{}, identity.ErrNotFound
	}
	var requestID uuid.UUID
	var status string
	err = tx.QueryRow(ctx, `SELECT id,status FROM media_requests WHERE media_id=$1 AND status IN ('requested','approved','queued','downloading','completed') ORDER BY CASE status WHEN 'completed' THEN 1 ELSE 0 END,created_at DESC LIMIT 1 FOR UPDATE`, mediaID).Scan(&requestID, &status)
	duplicate := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return MediaRequest{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		requestID = uuid.New()
		requestNo := "REQ-" + time.Now().UTC().Format("20060102") + "-" + strings.ToUpper(strings.ReplaceAll(requestID.String()[:8], "-", ""))
		if _, err = tx.Exec(ctx, `INSERT INTO media_requests(id,request_no,media_id,requested_by,note) VALUES($1,$2,$3,$4,NULLIF($5,''))`, requestID, requestNo, mediaID, accountID, note); err != nil {
			return MediaRequest{}, identity.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,to_status,actor,reason) VALUES($1,'created','requested',$2,$3)`, requestID, actor.Label(), note); err != nil {
			return MediaRequest{}, err
		}
		if err = emitAutomationEventTx(ctx, tx, "media_request.created:"+requestID.String(), "media_request.created", "media_request", requestID.String(), map[string]any{"media_request_id": requestID.String(), "media_id": mediaID.String(), "account_id": accountID.String()}); err != nil {
			return MediaRequest{}, err
		}
		var autoMatch bool
		_ = tx.QueryRow(ctx, `SELECT (value #>> '{}')::boolean FROM dynamic_settings WHERE key='media.auto_match_enabled'`).Scan(&autoMatch)
		if autoMatch {
			rows, queryErr := tx.Query(ctx, `SELECT id FROM emby_instances WHERE enabled ORDER BY priority,id`)
			if queryErr != nil {
				return MediaRequest{}, queryErr
			}
			var instanceIDs []uuid.UUID
			for rows.Next() {
				var instanceID uuid.UUID
				if queryErr = rows.Scan(&instanceID); queryErr != nil {
					rows.Close()
					return MediaRequest{}, queryErr
				}
				instanceIDs = append(instanceIDs, instanceID)
			}
			rows.Close()
			if queryErr = rows.Err(); queryErr != nil {
				return MediaRequest{}, queryErr
			}
			for _, instanceID := range instanceIDs {
				taskID := uuid.New()
				key := "media-match:" + mediaID.String() + ":" + instanceID.String() + ":request:" + requestID.String()
				if _, err = tx.Exec(ctx, `INSERT INTO media_matches(id,media_id,instance_id,status) VALUES($1,$2,$3,'pending') ON CONFLICT(media_id,instance_id) DO UPDATE SET status='pending',last_error=NULL,updated_at=NOW()`, uuid.New(), mediaID, instanceID); err != nil {
					return MediaRequest{}, err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'media.match',$2,$3,8,$4)`, taskID, key, jsonBytes(map[string]any{"instance_id": instanceID.String(), "media_id": mediaID.String()}), actor.Label()); err != nil {
					return MediaRequest{}, err
				}
			}
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO media_request_subscriptions(request_id,account_id,note) VALUES($1,$2,NULLIF($3,'')) ON CONFLICT DO NOTHING`, requestID, accountID, note)
	if err != nil {
		return MediaRequest{}, err
	}
	if tag.RowsAffected() == 1 && duplicate {
		if _, err = tx.Exec(ctx, `UPDATE media_requests SET subscriber_count=subscriber_count+1,updated_at=NOW() WHERE id=$1`, requestID); err != nil {
			return MediaRequest{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,actor,reason,details) VALUES($1,'subscribed',$2,$3,$4)`, requestID, actor.Label(), note, jsonBytes(map[string]any{"account_id": accountID})); err != nil {
			return MediaRequest{}, err
		}
	}
	if err = audit(ctx, tx, actor, "media_request.create", "media_request", requestID.String(), map[string]any{"duplicate": duplicate, "media_id": mediaID}); err != nil {
		return MediaRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaRequest{}, err
	}
	item, err := s.GetMediaRequest(ctx, requestID, &accountID)
	item.Duplicate = duplicate
	return item, err
}

func (s *Service) GetMediaRequest(ctx context.Context, id uuid.UUID, viewer *uuid.UUID) (MediaRequest, error) {
	var item MediaRequest
	var mediaID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT r.id,r.request_no,r.media_id,r.requested_by,r.status,r.priority,COALESCE(CASE WHEN $2::uuid IS NULL THEN r.note ELSE (SELECT x.note FROM media_request_subscriptions x WHERE x.request_id=r.id AND x.account_id=$2) END,''),r.subscriber_count,EXISTS(SELECT 1 FROM media_request_subscriptions x WHERE x.request_id=r.id AND x.account_id=$2),COALESCE(r.resolution_reason,''),COALESCE(r.resolved_by,''),r.resolved_at,r.revision,r.created_at,r.updated_at FROM media_requests r WHERE r.id=$1 AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM media_request_subscriptions x WHERE x.request_id=r.id AND x.account_id=$2))`, id, uuidQueryValue(viewer)).Scan(&item.ID, &item.RequestNo, &mediaID, &item.RequestedBy, &item.Status, &item.Priority, &item.Note, &item.SubscriberCount, &item.Subscribed, &item.ResolutionReason, &item.ResolvedBy, &item.ResolvedAt, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return MediaRequest{}, notFound(err)
	}
	item.Media, err = s.GetMedia(ctx, mediaID)
	return item, err
}

func (s *Service) ListMediaRequests(ctx context.Context, viewer *uuid.UUID, status string, limit int) ([]MediaRequest, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT r.id FROM media_requests r WHERE ($1='' OR r.status=$1) AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM media_request_subscriptions x WHERE x.request_id=r.id AND x.account_id=$2)) ORDER BY r.created_at DESC LIMIT $3`, status, uuidQueryValue(viewer), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]MediaRequest, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetMediaRequest(ctx, id, viewer)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) UpdateMediaRequestStatus(ctx context.Context, id uuid.UUID, status, reason string, expected int64, actor identity.Actor) (MediaRequest, error) {
	status, reason = normalize(status), strings.TrimSpace(reason)
	if !contains([]string{"approved", "queued", "downloading", "completed", "rejected", "canceled"}, status) || reason == "" || len(reason) > 1000 {
		return MediaRequest{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MediaRequest{}, err
	}
	defer tx.Rollback(ctx)
	var current string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT status,revision FROM media_requests WHERE id=$1 FOR UPDATE`, id).Scan(&current, &revision); err != nil {
		return MediaRequest{}, notFound(err)
	}
	if revision != expected || !validRequestTransition(current, status) {
		return MediaRequest{}, identity.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE media_requests SET status=$2::varchar,resolution_reason=$3,resolved_by=$4,resolved_at=CASE WHEN $2::varchar IN ('completed','rejected','canceled') THEN NOW() ELSE NULL END,revision=revision+1,updated_at=NOW() WHERE id=$1`, id, status, reason, actor.Label())
	if err != nil {
		return MediaRequest{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,from_status,to_status,actor,reason) VALUES($1,'status_changed',$2,$3,$4,$5)`, id, current, status, actor.Label(), reason); err != nil {
		return MediaRequest{}, err
	}
	if err = notifyRequestSubscribersTx(ctx, tx, id, "media_request.status", "求片状态已更新", reason); err != nil {
		return MediaRequest{}, err
	}
	if err = emitAutomationEventTx(ctx, tx, "media_request.status:"+id.String()+":"+fmt.Sprint(revision+1), "media_request.status_changed", "media_request", id.String(), map[string]any{"media_request_id": id.String(), "from_status": current, "to_status": status}); err != nil {
		return MediaRequest{}, err
	}
	if err = audit(ctx, tx, actor, "media_request.status", "media_request", id.String(), map[string]any{"from": current, "to": status, "reason": reason}); err != nil {
		return MediaRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaRequest{}, err
	}
	return s.GetMediaRequest(ctx, id, nil)
}

func validRequestTransition(from, to string) bool {
	allowed := map[string][]string{
		"requested":   {"approved", "rejected", "canceled", "completed"},
		"approved":    {"queued", "rejected", "canceled", "completed"},
		"queued":      {"downloading", "rejected", "canceled", "completed"},
		"downloading": {"completed", "rejected", "canceled"},
	}
	return contains(allowed[from], to)
}

func (s *Service) SearchMoviePilot(ctx context.Context, requestID uuid.UUID) ([]map[string]any, error) {
	request, err := s.GetMediaRequest(ctx, requestID, nil)
	if err != nil {
		return nil, err
	}
	credentialName := s.dynamicString(ctx, "moviepilot.credential_name", "moviepilot.api_token")
	token, err := s.credentialSecret(ctx, credentialName)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(s.dynamicString(ctx, "moviepilot.api_base_url", "http://moviepilot:3000"), "/")
	path := normalizedUpstreamPath(s.dynamicString(ctx, "moviepilot.search_path", "/api/v1/search/title"), "/api/v1/search/title")
	endpoint, err := url.Parse(base + path)
	if err != nil {
		return nil, identity.ErrInvalid
	}
	query := endpoint.Query()
	query.Set("keyword", request.Media.Title)
	endpoint.RawQuery = query.Encode()
	result, err := getJSON(ctx, endpoint.String(), token)
	if err != nil {
		return nil, fmt.Errorf("MoviePilot search failed: %w", err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return nil, fmt.Errorf("MoviePilot search rejected the request")
	}
	raw, ok := result["data"].([]any)
	if !ok {
		raw, _ = result["items"].([]any)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		if item, ok := value.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Service) SubmitMoviePilot(ctx context.Context, requestID uuid.UUID, resource map[string]any, idempotencyKey string, actor identity.Actor) (MoviePilotJob, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	resourceRaw, marshalErr := json.Marshal(resource)
	if idempotencyKey == "" || len(idempotencyKey) > 160 || marshalErr != nil || len(resourceRaw) > 512*1024 {
		return MoviePilotJob{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MoviePilotJob{}, err
	}
	defer tx.Rollback(ctx)
	var mediaID uuid.UUID
	var requestStatus string
	if err = tx.QueryRow(ctx, `SELECT media_id,status FROM media_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&mediaID, &requestStatus); err != nil {
		return MoviePilotJob{}, notFound(err)
	}
	if contains([]string{"completed", "rejected", "canceled"}, requestStatus) {
		return MoviePilotJob{}, identity.ErrConflict
	}
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM moviepilot_jobs WHERE media_id=$1 AND status IN ('pending','submitting','submitted','downloading','completed') ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, mediaID).Scan(&existing)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return MoviePilotJob{}, err
		}
		item, getErr := s.GetMoviePilotJob(ctx, existing)
		item.Duplicate = true
		return item, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MoviePilotJob{}, err
	}
	jobID, taskID := uuid.New(), uuid.New()
	fullKey := "moviepilot:" + mediaID.String() + ":" + idempotencyKey
	_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'moviepilot.submit',$2,$3,8,$4)`, taskID, fullKey, jsonBytes(map[string]any{"moviepilot_job_id": jobID.String()}), actor.Label())
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO moviepilot_jobs(id,media_id,request_id,task_id,idempotency_key,payload,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, jobID, mediaID, requestID, taskID, fullKey, jsonBytes(map[string]any{"request_id": requestID, "resource": resource}), actor.Label())
	}
	if err != nil {
		return MoviePilotJob{}, fmt.Errorf("queue moviepilot job: %w", err)
	}
	if requestStatus == "requested" || requestStatus == "approved" {
		if _, err = tx.Exec(ctx, `UPDATE media_requests SET status='queued',revision=revision+1,updated_at=NOW() WHERE id=$1`, requestID); err != nil {
			return MoviePilotJob{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,from_status,to_status,actor,reason,details) VALUES($1,'moviepilot_queued',$2,'queued',$3,'MoviePilot task queued',$4)`, requestID, requestStatus, actor.Label(), jsonBytes(map[string]any{"job_id": jobID, "task_id": taskID})); err != nil {
		return MoviePilotJob{}, err
	}
	if err = audit(ctx, tx, actor, "moviepilot.submit", "media_request", requestID.String(), map[string]any{"job_id": jobID}); err != nil {
		return MoviePilotJob{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MoviePilotJob{}, err
	}
	return s.GetMoviePilotJob(ctx, jobID)
}

func (s *Service) GetMoviePilotJob(ctx context.Context, id uuid.UUID) (MoviePilotJob, error) {
	var item MoviePilotJob
	var payload, result []byte
	err := s.db.QueryRow(ctx, `SELECT id,media_id,request_id,task_id,status,COALESCE(external_job_id,''),payload,result,attempts,COALESCE(last_error,''),submitted_at,completed_at,created_by,created_at,updated_at FROM moviepilot_jobs WHERE id=$1`, id).Scan(&item.ID, &item.MediaID, &item.RequestID, &item.TaskID, &item.Status, &item.ExternalJobID, &payload, &result, &item.Attempts, &item.LastError, &item.SubmittedAt, &item.CompletedAt, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	item.Payload, item.Result = decodeJSON(payload), decodeJSON(result)
	return item, notFound(err)
}

func (s *Service) ListMoviePilotJobs(ctx context.Context, status string, limit int) ([]MoviePilotJob, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM moviepilot_jobs WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MoviePilotJob
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		item, getErr := s.GetMoviePilotJob(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func notifyRequestSubscribersTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID, eventKey, title, body string) error {
	rows, err := tx.Query(ctx, `SELECT account_id FROM media_request_subscriptions WHERE request_id=$1`, requestID)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, accountID := range ids {
		if err = queuePreferredNotificationTx(ctx, tx, accountID, eventKey, title, body, map[string]any{"media_request_id": requestID}); err != nil {
			return err
		}
	}
	return nil
}

func emitAutomationEventTx(ctx context.Context, tx pgx.Tx, key, eventType, subjectType, subjectID string, payload map[string]any) error {
	_, err := tx.Exec(ctx, `INSERT INTO automation_events(id,event_key,event_type,subject_type,subject_id,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_key) DO NOTHING`, uuid.New(), key, eventType, subjectType, subjectID, jsonBytes(payload))
	return err
}

func floatValue(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	case int64:
		return float64(number)
	}
	return 0
}

func postJSON(ctx context.Context, endpoint, token string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", bearerToken(token))
	request.Header.Set("X-API-KEY", rawToken(token))
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("upstream returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	result := map[string]any{}
	if response.StatusCode != http.StatusNoContent {
		if err = json.NewDecoder(io.LimitReader(response.Body, maxEmbyResponse)).Decode(&result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func getJSON(ctx context.Context, endpoint, token string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", bearerToken(token))
	request.Header.Set("X-API-KEY", rawToken(token))
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("upstream returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	result := map[string]any{}
	if err = json.NewDecoder(io.LimitReader(response.Body, maxEmbyResponse)).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func bearerToken(token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return token
	}
	return "Bearer " + token
}

func rawToken(token string) string {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return strings.TrimSpace(token[len("bearer "):])
	}
	return token
}

func normalizedUpstreamPath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.Contains(value, "://") {
		return fallback
	}
	return value
}
