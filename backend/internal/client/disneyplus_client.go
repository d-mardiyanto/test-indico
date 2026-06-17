// Package client — Disney+ partner HTTP client.
// Disney+ uses a completely different wire format from NETPLAY and Netflix:
// email-based identity, tier-based plans, and token-based activation.
// Normalization to our internal contract happens in provider/disneyplus.go.
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

// DisneyPlusClient is a minimal HTTP client for the Disney+ partner API.
type DisneyPlusClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewDisneyPlusClient builds a client with the given timeout.
func NewDisneyPlusClient(baseURL string, timeout time.Duration) *DisneyPlusClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &DisneyPlusClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// ---- Wire DTOs (mirror Disney+ API exactly) ---------------------------------

type DisneyPlusSubscribeRequest struct {
	Email       string `json:"email"`
	Tier        string `json:"tier"`
	Region      string `json:"region"`
	ProfileName string `json:"profileName"`
}

type DisneyPlusSubscribeResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	AccessToken    string `json:"accessToken"`
	Status         string `json:"status"`
	Tier           string `json:"tier"`
	CreatedAt      string `json:"createdAt"`
	Message        string `json:"message,omitempty"`
}

type DisneyPlusActivateRequest struct {
	AccessToken string `json:"accessToken"`
}

type DisneyPlusActivateResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	Status         string `json:"status"`
	Tier           string `json:"tier"`
	ActivatedAt    string `json:"activatedAt"`
	ExpiresAt      string `json:"expiresAt"`
	Message        string `json:"message,omitempty"`
}

type DisneyPlusStatusResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	Tier           string `json:"tier"`
	ActivatedAt    string `json:"activatedAt"`
	ExpiresAt      string `json:"expiresAt"`
	Message        string `json:"message,omitempty"`
}

// ---- Calls ------------------------------------------------------------------

func (c *DisneyPlusClient) Subscribe(ctx context.Context, idempotencyKey string, req DisneyPlusSubscribeRequest) (*DisneyPlusSubscribeResponse, error) {
	var out DisneyPlusSubscribeResponse
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.do(ctx, http.MethodPost, "/api/v2/subscriptions", headers, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *DisneyPlusClient) Activate(ctx context.Context, token string) (*DisneyPlusActivateResponse, error) {
	var out DisneyPlusActivateResponse
	if err := c.do(ctx, http.MethodPost, "/api/v2/subscriptions/activate", nil, DisneyPlusActivateRequest{AccessToken: token}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *DisneyPlusClient) Status(ctx context.Context, token string) (*DisneyPlusStatusResponse, error) {
	var out DisneyPlusStatusResponse
	if err := c.do(ctx, http.MethodGet, "/api/v2/subscriptions/"+token, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *DisneyPlusClient) do(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
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
