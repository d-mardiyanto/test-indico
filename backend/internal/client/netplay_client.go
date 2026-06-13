// Package client contains thin HTTP wrappers around external partner APIs.
// Each client knows only the partner's wire format. Normalization to our
// internal contract happens in the provider layer.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// NetplayClient is a minimal HTTP client for the NETPLAY partner API.
type NetplayClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewNetplayClient builds a client with a sane default timeout. The caller
// can override HTTP to inject custom transports (e.g. in tests).
func NewNetplayClient(baseURL string, timeout time.Duration) *NetplayClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &NetplayClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// ---- Wire DTOs (mirror NETPLAY API exactly) ---------------------------------

type NetplaySubscribeRequest struct {
	UserID   string `json:"userId"`
	MSISDN   string `json:"msisdn"`
	Provider string `json:"provider"`
	Plan     string `json:"plan"`
}

type NetplaySubscribeResponse struct {
	SubscriptionRequestID string `json:"subscriptionRequestId"`
	ActivationToken       string `json:"activationToken"`
	Status                string `json:"status"`
	Message               string `json:"message,omitempty"`
}

type NetplayActivateRequest struct {
	ActivationToken string `json:"activationToken"`
}

type NetplayActivateResponse struct {
	Provider            string `json:"provider"`
	UserID              string `json:"userId"`
	ActivationStatus    string `json:"activationStatus"`
	SubscriptionStatus  string `json:"subscriptionStatus"`
	Plan                string `json:"plan"`
	ExternalReferenceID string `json:"externalReferenceId"`
	ActivatedAt         string `json:"activatedAt"`
	Message             string `json:"message,omitempty"`
}

type NetplayStatusResponse struct {
	SubscriptionRequestID string `json:"subscriptionRequestId"`
	UserID                string `json:"userId"`
	Provider              string `json:"provider"`
	Plan                  string `json:"plan"`
	SubscriptionStatus    string `json:"subscriptionStatus"`
	ActivatedAt           string `json:"activatedAt"`
	TokenExpiresAt        string `json:"tokenExpiresAt"`
	ExternalReferenceID   string `json:"externalReferenceId"`
	Message               string `json:"message,omitempty"`
}

// HTTPError carries the upstream status code so the provider layer can map it
// to a typed error (timeout/unavailable/unauthorized/...).
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("netplay http %d: %s", e.StatusCode, e.Body)
}

// ---- Calls ------------------------------------------------------------------

func (c *NetplayClient) Subscribe(ctx context.Context, idempotencyKey string, req NetplaySubscribeRequest) (*NetplaySubscribeResponse, error) {
	var out NetplaySubscribeResponse
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	if err := c.do(ctx, http.MethodPost, "/subscribe", headers, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *NetplayClient) Activate(ctx context.Context, token string) (*NetplayActivateResponse, error) {
	var out NetplayActivateResponse
	if err := c.do(ctx, http.MethodPost, "/activate", nil, NetplayActivateRequest{ActivationToken: token}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *NetplayClient) Status(ctx context.Context, token string) (*NetplayStatusResponse, error) {
	var out NetplayStatusResponse
	path := "/subscription-status?activationToken=" + url.QueryEscape(token)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do performs the request, JSON-encoding body (if any) and decoding into out
// when the response is 2xx. Non-2xx returns *HTTPError; transport/timeout
// errors are returned as-is.
func (c *NetplayClient) do(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
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

// AsHTTPError tries to extract an *HTTPError from err. Convenience for the
// provider layer when mapping wire errors to typed errors.
func AsHTTPError(err error) (*HTTPError, bool) {
	var he *HTTPError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}
