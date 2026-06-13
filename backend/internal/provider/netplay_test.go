package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/client"
	"backend/internal/model"
)

// newTestServer spins up an httptest.Server that returns the given status code
// and JSON body for any request, plus a NetplayClient pointed at it.
func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *client.NetplayClient) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, client.NewNetplayClient(srv.URL, 2*time.Second)
}

func TestNetplayProvider_Subscribe_Normalization(t *testing.T) {
	body := `{
		"subscriptionRequestId": "SUBREQ-F52CBE2C",
		"activationToken": "tok-123",
		"status": "pending_activation"
	}`
	_, c := newTestServer(t, 200, body)
	p := NewNetplayProviderWithAPI(c)

	resp, err := p.Subscribe(context.Background(), SubscribeRequest{
		UserID:         "user-1",
		MSISDN:         "62800",
		Plan:           "PREMIUM_30D",
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProviderRequestID != "SUBREQ-F52CBE2C" {
		t.Errorf("provider request id: got %q", resp.ProviderRequestID)
	}
	if resp.ActivationToken != "tok-123" {
		t.Errorf("activation token: got %q", resp.ActivationToken)
	}
	if resp.Status != model.StatusPendingActivation {
		t.Errorf("status: got %q, want %q", resp.Status, model.StatusPendingActivation)
	}
}

func TestNetplayProvider_Activate_Normalization(t *testing.T) {
	body := `{
		"provider": "NETPLAY",
		"userId": "user-1",
		"activationStatus": "success",
		"subscriptionStatus": "active",
		"plan": "PREMIUM_30D",
		"externalReferenceId": "EXT-1",
		"activatedAt": "2026-05-17T16:29:33Z",
		"message": "ok"
	}`
	_, c := newTestServer(t, 200, body)
	p := NewNetplayProviderWithAPI(c)

	resp, err := p.Activate(context.Background(), "tok-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != model.StatusActive {
		t.Errorf("status: got %q, want %q", resp.Status, model.StatusActive)
	}
	if resp.ExternalReferenceID != "EXT-1" {
		t.Errorf("externalReferenceId: got %q", resp.ExternalReferenceID)
	}
	if resp.ActivatedAt == nil || resp.ActivatedAt.Year() != 2026 {
		t.Errorf("activatedAt not parsed: %+v", resp.ActivatedAt)
	}
}

func TestNetplayProvider_Status_Normalization(t *testing.T) {
	body := `{
		"subscriptionRequestId": "SUBREQ-1",
		"userId": "user-1",
		"provider": "NETPLAY",
		"plan": "PREMIUM_30D",
		"subscriptionStatus": "active",
		"activatedAt": "2026-05-17T16:29:33Z",
		"tokenExpiresAt": "2026-05-20T16:29:04Z",
		"externalReferenceId": "EXT-1"
	}`
	_, c := newTestServer(t, 200, body)
	p := NewNetplayProviderWithAPI(c)

	resp, err := p.Status(context.Background(), "tok-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != model.StatusActive {
		t.Errorf("status: got %q", resp.Status)
	}
	if resp.TokenExpiresAt == nil {
		t.Errorf("tokenExpiresAt nil")
	}
}

func TestNetplayProvider_ErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"401_unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"403_unauthorized", http.StatusForbidden, ErrUnauthorized},
		{"404_not_found", http.StatusNotFound, ErrNotFound},
		{"500_unavailable", http.StatusInternalServerError, ErrUnavailable},
		{"502_unavailable", http.StatusBadGateway, ErrUnavailable},
		{"400_bad", http.StatusBadRequest, ErrBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newTestServer(t, tc.status, `{"error":"x"}`)
			p := NewNetplayProviderWithAPI(c)
			_, err := p.Activate(context.Background(), "tok")
			if err == nil {
				t.Fatal("expected error")
			}
			if err != tc.want {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNetplayProvider_TimeoutMapping(t *testing.T) {
	// Server that hangs longer than our context deadline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	c := client.NewNetplayClient(srv.URL, 2*time.Second)
	p := NewNetplayProviderWithAPI(c)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := p.Activate(ctx, "tok")
	if err != ErrTimeout {
		t.Errorf("got %v, want %v", err, ErrTimeout)
	}
}

func TestNetplayProvider_Subscribe_SendsIdempotencyKey(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"subscriptionRequestId":"x","activationToken":"y","status":"pending_activation"}`))
	}))
	defer srv.Close()
	c := client.NewNetplayClient(srv.URL, 2*time.Second)
	p := NewNetplayProviderWithAPI(c)

	_, err := p.Subscribe(context.Background(), SubscribeRequest{
		UserID: "u", MSISDN: "m", Plan: "p", IdempotencyKey: "MY-KEY",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "MY-KEY" {
		t.Errorf("Idempotency-Key header: got %q", got)
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"active":             model.StatusActive,
		"ACTIVE":             model.StatusActive,
		"pending_activation": model.StatusPendingActivation,
		"pending":            model.StatusPendingActivation,
		"failed":             model.StatusFailed,
		"activation_failed":  model.StatusFailed,
		"expired":            model.StatusExpired,
		"":                   model.StatusUnknown,
		"weird-value":        model.StatusUnknown,
	}
	for in, want := range cases {
		if got := normalizeStatus(in); got != want {
			t.Errorf("normalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

// Suppress unused import warning for strings if go decides to optimize away.
var _ = strings.TrimSpace
