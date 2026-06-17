package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"backend/internal/client"
	"backend/internal/model"
)

const providerNameDisneyPlus = "DISNEYPLUS"

// disneyPlusAPI is the subset of client.DisneyPlusClient we depend on.
type disneyPlusAPI interface {
	Subscribe(ctx context.Context, idempotencyKey string, req client.DisneyPlusSubscribeRequest) (*client.DisneyPlusSubscribeResponse, error)
	Activate(ctx context.Context, token string) (*client.DisneyPlusActivateResponse, error)
	Status(ctx context.Context, token string) (*client.DisneyPlusStatusResponse, error)
}

// DisneyPlusProvider implements Provider by adapting Disney+'s wire format to
// our normalized contract.
type DisneyPlusProvider struct {
	api disneyPlusAPI
}

// NewDisneyPlusProvider builds a DisneyPlusProvider backed by a real HTTP client.
func NewDisneyPlusProvider(baseURL string, timeout time.Duration) *DisneyPlusProvider {
	return &DisneyPlusProvider{api: client.NewDisneyPlusClient(baseURL, timeout)}
}

// NewDisneyPlusProviderWithAPI is exposed for tests / dependency injection.
func NewDisneyPlusProviderWithAPI(api disneyPlusAPI) *DisneyPlusProvider {
	return &DisneyPlusProvider{api: api}
}

func (p *DisneyPlusProvider) Name() string { return providerNameDisneyPlus }

func (p *DisneyPlusProvider) Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResponse, error) {
	email := req.Extras["accountEmail"]
	tier := req.Extras["subscriptionTier"]
	region := req.Extras["region"]
	profileName := req.Extras["profileName"]

	if email == "" {
		return nil, fmt.Errorf("%w: accountEmail is required for DISNEYPLUS", ErrBadResponse)
	}
	if tier == "" {
		tier = req.Plan
	}

	raw, err := p.api.Subscribe(ctx, req.IdempotencyKey, client.DisneyPlusSubscribeRequest{
		Email:       email,
		Tier:        tier,
		Region:      region,
		ProfileName: profileName,
	})
	if err != nil {
		return nil, mapDisneyPlusError(err)
	}
	return &SubscribeResponse{
		ProviderRequestID: raw.SubscriptionID,
		ActivationToken:   raw.AccessToken,
		Status:            normalizeDisneyPlusStatus(raw.Status),
		RawMessage:        raw.Message,
	}, nil
}

func (p *DisneyPlusProvider) Activate(ctx context.Context, token string) (*ActivateResponse, error) {
	raw, err := p.api.Activate(ctx, token)
	if err != nil {
		return nil, mapDisneyPlusError(err)
	}
	return &ActivateResponse{
		Status:              normalizeDisneyPlusStatus(raw.Status),
		Plan:                raw.Tier,
		ExternalReferenceID: raw.SubscriptionID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		RawMessage:          raw.Message,
	}, nil
}

func (p *DisneyPlusProvider) Status(ctx context.Context, token string) (*StatusResponse, error) {
	raw, err := p.api.Status(ctx, token)
	if err != nil {
		return nil, mapDisneyPlusError(err)
	}
	return &StatusResponse{
		ProviderRequestID:   raw.SubscriptionID,
		UserID:              raw.Email,
		Plan:                raw.Tier,
		Status:              normalizeDisneyPlusStatus(raw.Status),
		ExternalReferenceID: raw.SubscriptionID,
		ActivatedAt:         parseTime(raw.ActivatedAt),
		TokenExpiresAt:      parseTime(raw.ExpiresAt),
		RawMessage:          raw.Message,
	}, nil
}

// normalizeDisneyPlusStatus maps Disney+ status strings to our canonical model.Status*.
func normalizeDisneyPlusStatus(s string) string {
	switch s {
	case "ACTIVE", "active":
		return model.StatusActive
	case "PENDING", "pending", "PENDING_ACTIVATION", "PENDING_VERIFICATION", "AWAITING_ACTIVATION":
		return model.StatusPendingActivation
	case "FAILED", "failed", "REJECTED", "rejected":
		return model.StatusFailed
	case "EXPIRED", "expired", "CANCELLED", "cancelled":
		return model.StatusExpired
	case "":
		return model.StatusUnknown
	default:
		return model.StatusUnknown
	}
}

func mapDisneyPlusError(err error) error {
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
