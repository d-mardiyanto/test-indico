// Package client — Netflix partner HTTP client.
// Netflix uses different field names and endpoint paths from NETPLAY; this
// client mirrors that wire format exactly. Normalization happens in the
// provider layer (internal/provider/netflix.go).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NetflixClient is a minimal HTTP client for the Netflix partner API.
type NetflixClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewNetflixClient builds a client with the given timeout.
func NewNetflixClient(baseURL string, timeout time.Duration) *NetflixClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &NetflixClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// ---- Wire DTOs (mirror Netflix API exactly) ---------------------------------

type NetflixSubscribeRequest struct {
	ExternalUserID string `json:"externalUserId"`
	PhoneNumber    string `json:"phoneNumber"`
	ContentPlan    string `json:"contentPlan"`
}

type NetflixSubscribeResponse struct {
	ReferenceID  string `json:"referenceId"`
	ContentToken string `json:"contentToken"`
	State        string `json:"state"`
	Info         string `json:"info,omitempty"`
}

type NetflixActivateRequest struct {
	ContentToken string `json:"contentToken"`
}

type NetflixActivateResponse struct {
	ReferenceID    string `json:"referenceId"`
	SubscriptionID string `json:"subscriptionId"`
	State          string `json:"state"`
	ContentPlan    string `json:"contentPlan"`
	ActivatedAt    string `json:"activatedAt"`
	Info           string `json:"info,omitempty"`
}

type NetflixStatusResponse struct {
	ReferenceID    string `json:"referenceId"`
	ExternalUserID string `json:"externalUserId"`
	State          string `json:"state"`
	ContentPlan    string `json:"contentPlan"`
	ActivatedAt    string `json:"activatedAt"`
	ValidUntil     string `json:"validUntil"`
	SubscriptionID string `json:"subscriptionId"`
	Info           string `json:"info,omitempty"`
}

// ---- Calls ------------------------------------------------------------------

func (c *NetflixClient) Subscribe(ctx context.Context, idempotencyKey string, req NetflixSubscribeRequest) (*NetflixSubscribeResponse, error) {
	var out NetflixSubscribeResponse
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.do(ctx, http.MethodPost, "/subscription", headers, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *NetflixClient) Activate(ctx context.Context, token string) (*NetflixActivateResponse, error) {
	var out NetflixActivateResponse
	if err := c.do(ctx, http.MethodPost, "/activation", nil, NetflixActivateRequest{ContentToken: token}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *NetflixClient) Status(ctx context.Context, token string) (*NetflixStatusResponse, error) {
	var out NetflixStatusResponse
	if err := c.do(ctx, http.MethodGet, "/subscription/"+token, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *NetflixClient) do(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
