package provider

import (
	"context"
	"errors"
	"net/http"
	"time"

	"backend/internal/client"
	"backend/internal/model"
)

const providerNameNetflix = "NETFLIX"

// netflixAPI is the subset of client.NetflixClient we depend on. Defining it
// here lets us swap in fakes during tests.
type netflixAPI interface {
	Subscribe(ctx context.Context, idempotencyKey string, req client.NetflixSubscribeRequest) (*client.NetflixSubscribeResponse, error)
	Activate(ctx context.Context, token string) (*client.NetflixActivateResponse, error)
	Status(ctx context.Context, token string) (*client.NetflixStatusResponse, error)
}

// NetflixProvider implements Provider by adapting Netflix's wire format to
// our normalized contract.
type NetflixProvider struct {
	api netflixAPI
}

// NewNetflixProvider builds a NetflixProvider backed by a real HTTP client.
func NewNetflixProvider(baseURL string, timeout time.Duration) *NetflixProvider {
	return &NetflixProvider{api: client.NewNetflixClient(baseURL, timeout)}
}

// NewNetflixProviderWithAPI is exposed for tests / dependency injection.
func NewNetflixProviderWithAPI(api netflixAPI) *NetflixProvider {
	return &NetflixProvider{api: api}
}

func (p *NetflixProvider) Name() string { return providerNameNetflix }

func (p *NetflixProvider) Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResponse, error) {
	raw, err := p.api.Subscribe(ctx, req.IdempotencyKey, client.NetflixSubscribeRequest{
		ExternalUserID: req.UserID,
		PhoneNumber:    req.MSISDN,
		ContentPlan:    req.Plan,
	})
	if err != nil {
		return nil, mapNetflixError(err)
	}
	return &SubscribeResponse{
		ProviderRequestID: raw.ReferenceID,
		ActivationToken:   raw.ContentToken,
		Status:            normalizeNetflixStatus(raw.State),
		RawMessage:        raw.Info,
	}, nil
}

func (p *NetflixProvider) Activate(ctx context.Context, token string) (*ActivateResponse, error) {
	raw, err := p.api.Activate(ctx, token)
	if err != nil {
		return nil, mapNetflixError(err)
	}
	return &ActivateResponse{
		Status:              normalizeNetflixStatus(raw.State),
		Plan:                raw.ContentPlan,
		ExternalReferenceID: raw.SubscriptionID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		RawMessage:          raw.Info,
	}, nil
}

func (p *NetflixProvider) Status(ctx context.Context, token string) (*StatusResponse, error) {
	raw, err := p.api.Status(ctx, token)
	if err != nil {
		return nil, mapNetflixError(err)
	}
	return &StatusResponse{
		ProviderRequestID:   raw.ReferenceID,
		UserID:              raw.ExternalUserID,
		Plan:                raw.ContentPlan,
		Status:              normalizeNetflixStatus(raw.State),
		ExternalReferenceID: raw.SubscriptionID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		TokenExpiresAt:      parseTime(raw.ValidUntil),
		RawMessage:          raw.Info,
	}, nil
}

// normalizeNetflixStatus maps Netflix state strings to our canonical model.Status*.
func normalizeNetflixStatus(s string) string {
	switch s {
	case "activated", "ACTIVATED", "active", "ACTIVE":
		return model.StatusActive
	case "awaiting_activation", "AWAITING_ACTIVATION", "pending", "processing":
		return model.StatusPendingActivation
	case "failed", "FAILED", "rejected", "REJECTED":
		return model.StatusFailed
	case "expired", "EXPIRED", "cancelled", "CANCELLED":
		return model.StatusExpired
	case "":
		return model.StatusUnknown
	default:
		return model.StatusUnknown
	}
}

func mapNetflixError(err error) error {
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
	return ErrUnavailable
}
