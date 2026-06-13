package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/model"
	"backend/internal/provider"
	"backend/internal/storage"
)

// fakeProvider implements provider.Provider for service-level testing.
type fakeProvider struct {
	name       string
	subscribeF func(ctx context.Context, req provider.SubscribeRequest) (*provider.SubscribeResponse, error)
	activateF  func(ctx context.Context, token string) (*provider.ActivateResponse, error)
	statusF    func(ctx context.Context, token string) (*provider.StatusResponse, error)
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Subscribe(ctx context.Context, req provider.SubscribeRequest) (*provider.SubscribeResponse, error) {
	return f.subscribeF(ctx, req)
}
func (f *fakeProvider) Activate(ctx context.Context, token string) (*provider.ActivateResponse, error) {
	return f.activateF(ctx, token)
}
func (f *fakeProvider) Status(ctx context.Context, token string) (*provider.StatusResponse, error) {
	if f.statusF == nil {
		return &provider.StatusResponse{Status: model.StatusUnknown}, nil
	}
	return f.statusF(ctx, token)
}

func newSvc(t *testing.T, fp *fakeProvider) *SubscriptionService {
	t.Helper()
	reg := provider.NewRegistry(fp)
	store := storage.NewMemoryStorage()
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return NewSubscriptionService(Config{
		Registry:        reg,
		Storage:         store,
		FrontendBaseURL: "http://app.test",
		Now:             func() time.Time { return fixed },
		GenCode:         func() (string, error) { return "ABC123", nil },
	})
}

func TestSubscribe_HappyPath(t *testing.T) {
	fp := &fakeProvider{
		name: "NETPLAY",
		subscribeF: func(ctx context.Context, req provider.SubscribeRequest) (*provider.SubscribeResponse, error) {
			if req.IdempotencyKey == "" {
				t.Error("idempotency key should be set")
			}
			return &provider.SubscribeResponse{
				ProviderRequestID: "SUBREQ-1",
				ActivationToken:   "tok-1",
				Status:            model.StatusPendingActivation,
			}, nil
		},
	}
	svc := newSvc(t, fp)

	res, err := svc.Subscribe(context.Background(), SubscribeInput{
		UserID: "u1", MSISDN: "62800", Provider: "NETPLAY", Plan: "PREMIUM_30D",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.Subscription.ActivationCode != "ABC123" {
		t.Errorf("activation code: got %q", res.Subscription.ActivationCode)
	}
	if res.ActivationLink != "http://app.test/activation/ABC123" {
		t.Errorf("activation link: got %q", res.ActivationLink)
	}
	if res.Subscription.SubscriptionStatus != model.StatusPendingActivation {
		t.Errorf("status: got %q", res.Subscription.SubscriptionStatus)
	}
	if !contains(res.SMSMessage, "NETPLAY") || !contains(res.SMSMessage, res.ActivationLink) {
		t.Errorf("sms message missing pieces: %q", res.SMSMessage)
	}
}

func TestSubscribe_UnknownProvider(t *testing.T) {
	svc := newSvc(t, &fakeProvider{name: "NETPLAY"})
	_, err := svc.Subscribe(context.Background(), SubscribeInput{
		UserID: "u", MSISDN: "m", Provider: "DOES_NOT_EXIST", Plan: "p",
	})
	if !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("got %v, want ErrUnknownProvider", err)
	}
}

func TestSubscribe_InvalidRequest(t *testing.T) {
	svc := newSvc(t, &fakeProvider{name: "NETPLAY"})
	_, err := svc.Subscribe(context.Background(), SubscribeInput{Provider: "NETPLAY"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("got %v, want ErrInvalidRequest", err)
	}
}

func TestActivate_HappyPath(t *testing.T) {
	activated := time.Date(2026, 5, 17, 16, 29, 33, 0, time.UTC)
	fp := &fakeProvider{
		name: "NETPLAY",
		subscribeF: func(ctx context.Context, _ provider.SubscribeRequest) (*provider.SubscribeResponse, error) {
			return &provider.SubscribeResponse{ActivationToken: "tok-1", Status: model.StatusPendingActivation}, nil
		},
		activateF: func(ctx context.Context, token string) (*provider.ActivateResponse, error) {
			if token != "tok-1" {
				t.Errorf("token: got %q", token)
			}
			return &provider.ActivateResponse{
				Status:              model.StatusActive,
				Plan:                "PREMIUM_30D",
				ExternalReferenceID: "EXT-1",
				ActivatedAt:         &activated,
				RawMessage:          "ok",
			}, nil
		},
	}
	svc := newSvc(t, fp)

	res, err := svc.Subscribe(context.Background(), SubscribeInput{
		UserID: "u", MSISDN: "m", Provider: "NETPLAY", Plan: "PREMIUM_30D",
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	sub, err := svc.Activate(context.Background(), res.Subscription.ActivationCode)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if sub.SubscriptionStatus != model.StatusActive {
		t.Errorf("status: got %q", sub.SubscriptionStatus)
	}
	if sub.ExternalReferenceID != "EXT-1" {
		t.Errorf("ext ref: got %q", sub.ExternalReferenceID)
	}
}

func TestActivate_NotFound(t *testing.T) {
	svc := newSvc(t, &fakeProvider{name: "NETPLAY"})
	_, err := svc.Activate(context.Background(), "NOPE12")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestActivate_Idempotent_AlreadyActive(t *testing.T) {
	calls := 0
	fp := &fakeProvider{
		name: "NETPLAY",
		subscribeF: func(ctx context.Context, _ provider.SubscribeRequest) (*provider.SubscribeResponse, error) {
			return &provider.SubscribeResponse{ActivationToken: "tok", Status: model.StatusPendingActivation}, nil
		},
		activateF: func(ctx context.Context, _ string) (*provider.ActivateResponse, error) {
			calls++
			return &provider.ActivateResponse{Status: model.StatusActive}, nil
		},
	}
	svc := newSvc(t, fp)
	res, _ := svc.Subscribe(context.Background(), SubscribeInput{
		UserID: "u", MSISDN: "m", Provider: "NETPLAY", Plan: "p",
	})
	if _, err := svc.Activate(context.Background(), res.Subscription.ActivationCode); err != nil {
		t.Fatalf("activate 1: %v", err)
	}
	if _, err := svc.Activate(context.Background(), res.Subscription.ActivationCode); err != nil {
		t.Fatalf("activate 2: %v", err)
	}
	if calls != 1 {
		t.Errorf("provider.Activate calls: got %d, want 1 (second call should short-circuit)", calls)
	}
}

func TestActivate_ProviderFailureSurfaced(t *testing.T) {
	fp := &fakeProvider{
		name: "NETPLAY",
		subscribeF: func(ctx context.Context, _ provider.SubscribeRequest) (*provider.SubscribeResponse, error) {
			return &provider.SubscribeResponse{ActivationToken: "tok", Status: model.StatusPendingActivation}, nil
		},
		activateF: func(ctx context.Context, _ string) (*provider.ActivateResponse, error) {
			return nil, provider.ErrUnavailable
		},
	}
	svc := newSvc(t, fp)
	res, _ := svc.Subscribe(context.Background(), SubscribeInput{
		UserID: "u", MSISDN: "m", Provider: "NETPLAY", Plan: "p",
	})
	_, err := svc.Activate(context.Background(), res.Subscription.ActivationCode)
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
