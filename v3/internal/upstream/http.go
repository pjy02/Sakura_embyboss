package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPProbe struct {
	client *http.Client
	url    string
}

func NewHTTPProbe(baseURL string, timeout time.Duration) *HTTPProbe {
	return &HTTPProbe{
		client: &http.Client{Timeout: timeout},
		url:    strings.TrimRight(baseURL, "/") + "/health/ready",
	}
}

func (p *HTTPProbe) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream readiness returned %s", response.Status)
	}
	return nil
}
