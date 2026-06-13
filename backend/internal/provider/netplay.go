package provider

import (
	"context"
	"errors"
	"net/http"
	"time"

	"backend/internal/client"
	"backend/internal/model"
)

const providerNameNetplay = "NETPLAY"

// netplayAPI is the subset of client.NetplayClient we depend on. Defining it
// here lets us swap in fakes during tests.
type netplayAPI interface {
	Subscribe(ctx context.Context, idempotencyKey string, req client.NetplaySubscribeRequest) (*client.NetplaySubscribeResponse, error)
	Activate(ctx context.Context, token string) (*client.NetplayActivateResponse, error)
	Status(ctx context.Context, token string) (*client.NetplayStatusResponse, error)
}

// NetplayProvider implements Provider by adapting NETPLAY's wire format to
// our normalized contract.
type NetplayProvider struct {
	api netplayAPI
}

// NewNetplayProvider builds a NetplayProvider backed by a real HTTP client.
func NewNetplayProvider(baseURL string, timeout time.Duration) *NetplayProvider {
	return &NetplayProvider{api: client.NewNetplayClient(baseURL, timeout)}
}

// NewNetplayProviderWithAPI is exposed for tests / dependency injection.
func NewNetplayProviderWithAPI(api netplayAPI) *NetplayProvider {
	return &NetplayProvider{api: api}
}

func (p *NetplayProvider) Name() string { return providerNameNetplay }

func (p *NetplayProvider) Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResponse, error) {
	raw, err := p.api.Subscribe(ctx, req.IdempotencyKey, client.NetplaySubscribeRequest{
		UserID:   req.UserID,
		MSISDN:   req.MSISDN,
		Provider: providerNameNetplay,
		Plan:     req.Plan,
	})
	if err != nil {
		return nil, mapNetplayError(err)
	}
	return &SubscribeResponse{
		ProviderRequestID: raw.SubscriptionRequestID,
		ActivationToken:   raw.ActivationToken,
		Status:            NormalizeStatus(raw.Status),
		RawMessage:        raw.Message,
	}, nil
}

func (p *NetplayProvider) Activate(ctx context.Context, token string) (*ActivateResponse, error) {
	raw, err := p.api.Activate(ctx, token)
	if err != nil {
		return nil, mapNetplayError(err)
	}
	status := NormalizeStatus(raw.SubscriptionStatus)
	// Treat explicit activation failure as failed even if subscriptionStatus is empty.
	if raw.ActivationStatus != "" && raw.ActivationStatus != "success" && status == model.StatusUnknown {
		status = model.StatusFailed
	}
	return &ActivateResponse{
		Status:              status,
		Plan:                raw.Plan,
		ExternalReferenceID: raw.ExternalReferenceID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		RawMessage:          raw.Message,
	}, nil
}

func (p *NetplayProvider) Status(ctx context.Context, token string) (*StatusResponse, error) {
	raw, err := p.api.Status(ctx, token)
	if err != nil {
		return nil, mapNetplayError(err)
	}
	return &StatusResponse{
		ProviderRequestID:   raw.SubscriptionRequestID,
		UserID:              raw.UserID,
		Plan:                raw.Plan,
		Status:              NormalizeStatus(raw.SubscriptionStatus),
		ExternalReferenceID: raw.ExternalReferenceID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		TokenExpiresAt:      parseTime(raw.TokenExpiresAt),
		RawMessage:          raw.Message,
	}, nil
}

// NormalizeStatus maps NETPLAY status strings to our canonical model.Status*.
func NormalizeStatus(s string) string {
	switch s {
	case "active", "ACTIVE":
		return model.StatusActive
	case "pending_activation", "PENDING_ACTIVATION", "pending":
		return model.StatusPendingActivation
	case "failed", "FAILED", "activation_failed":
		return model.StatusFailed
	case "expired", "EXPIRED":
		return model.StatusExpired
	case "":
		return model.StatusUnknown
	default:
		return model.StatusUnknown
	}
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

// mapNetplayError converts low-level client errors into typed provider errors
// so the service layer can decide how to react (e.g. surface 5xx vs 4xx).
func mapNetplayError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrTimeout
	}
	if he, ok := client.AsHTTPError(err); ok {
		switch {
		case he.StatusCode == http.StatusUnauthorized, he.StatusCode == http.StatusForbidden:
			return ErrUnauthorized
		case he.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case he.StatusCode >= 500:
			return ErrUnavailable
		default:
			return ErrBadResponse
		}
	}
	// Network / transport errors fall here.
	return ErrUnavailable
}
