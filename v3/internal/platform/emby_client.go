package platform

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxEmbyResponse = 8 << 20

type embyClient struct {
	baseURL *url.URL
	token   string
	client  *http.Client
}

type embySystemInfo struct {
	ID         string `json:"Id"`
	ServerName string `json:"ServerName"`
	Version    string `json:"Version"`
}

type embyUser struct {
	ID     string         `json:"Id"`
	Name   string         `json:"Name"`
	Policy map[string]any `json:"Policy"`
	Raw    map[string]any `json:"-"`
}

func (u embyUser) disabled() bool {
	value, _ := u.Policy["IsDisabled"].(bool)
	return value
}

func newEmbyClient(instance EmbyInstance, token string) (*embyClient, error) {
	parsed, err := url.Parse(strings.TrimRight(instance.BaseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || token == "" {
		return nil, errors.New("invalid Emby connection configuration")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !instance.VerifyTLS} // #nosec G402 -- explicitly controlled per instance.
	return &embyClient{baseURL: parsed, token: token, client: &http.Client{Timeout: 20 * time.Second, Transport: transport}}, nil
}

func (c *embyClient) endpoint(fragment string) string {
	copyURL := *c.baseURL
	copyURL.Path = path.Join(c.baseURL.Path, "emby", fragment)
	return copyURL.String()
}

func (c *embyClient) request(ctx context.Context, method, fragment string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(fragment), reader)
	if err != nil {
		return err
	}
	request.Header.Set("X-Emby-Token", c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("Emby request failed: %w", sanitizeURLError(err))
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxEmbyResponse)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(limited, 4096))
		return fmt.Errorf("Emby returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}
	if err = json.NewDecoder(limited).Decode(target); err != nil {
		return fmt.Errorf("invalid Emby response: %w", err)
	}
	return nil
}

func sanitizeURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (c *embyClient) probe(ctx context.Context) (embySystemInfo, time.Duration, error) {
	started := time.Now()
	var info embySystemInfo
	err := c.request(ctx, http.MethodGet, "System/Info", nil, &info)
	if err != nil {
		return embySystemInfo{}, time.Since(started), err
	}
	if info.ID == "" {
		return embySystemInfo{}, time.Since(started), errors.New("Emby system information has no server id")
	}
	return info, time.Since(started), nil
}

func (c *embyClient) users(ctx context.Context) ([]embyUser, error) {
	var raw []map[string]any
	if err := c.request(ctx, http.MethodGet, "Users", nil, &raw); err != nil {
		return nil, err
	}
	users := make([]embyUser, 0, len(raw))
	for _, item := range raw {
		encoded, _ := json.Marshal(item)
		var user embyUser
		_ = json.Unmarshal(encoded, &user)
		user.Raw = item
		if user.ID != "" && user.Name != "" {
			users = append(users, user)
		}
	}
	return users, nil
}

func (c *embyClient) createUser(ctx context.Context, username string) (embyUser, error) {
	var user embyUser
	if err := c.request(ctx, http.MethodPost, "Users/New", map[string]string{"Name": username}, &user); err != nil {
		return embyUser{}, err
	}
	if user.ID == "" {
		return embyUser{}, errors.New("Emby created a user without an id")
	}
	encoded, _ := json.Marshal(user)
	_ = json.Unmarshal(encoded, &user.Raw)
	return user, nil
}

func (c *embyClient) setPassword(ctx context.Context, userID, password string) error {
	return c.request(ctx, http.MethodPost, "Users/"+url.PathEscape(userID)+"/Password", map[string]any{"Id": userID, "NewPw": password, "ResetPassword": true}, nil)
}

func (c *embyClient) setDisabled(ctx context.Context, user embyUser, disabled bool) error {
	policy := user.Policy
	if policy == nil {
		policy = map[string]any{}
	}
	policy["IsDisabled"] = disabled
	return c.request(ctx, http.MethodPost, "Users/"+url.PathEscape(user.ID)+"/Policy", policy, nil)
}
