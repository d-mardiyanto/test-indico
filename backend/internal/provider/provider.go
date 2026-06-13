package provider

import (
	"context"
	"errors"
	"time"
)

// Provider abstracts an external OTT partner. Implementations must normalize
// partner-specific responses into these shared DTOs so upper layers stay
// provider-agnostic.
type Provider interface {
	Name() string

	Subscribe(ctx context.Context, req SubscribeRequest) (*SubscribeResponse, error)
	Activate(ctx context.Context, token string) (*ActivateResponse, error)
	Status(ctx context.Context, token string) (*StatusResponse, error)
}

// SubscribeRequest is the normalized input to a provider's subscribe call.
type SubscribeRequest struct {
	UserID         string
	MSISDN         string
	Plan           string
	IdempotencyKey string
}

// SubscribeResponse is the normalized subscribe result.
type SubscribeResponse struct {
	ProviderRequestID string // e.g. SUBREQ-XXXX
	ActivationToken   string
	Status            string // normalized status (model.Status*)
	RawMessage        string
}

// ActivateResponse is the normalized activate result.
type ActivateResponse struct {
	Status              string // normalized status
	Plan                string
	ExternalReferenceID string
	ActivatedAt         *time.Time
	RawMessage          string
}

// StatusResponse is the normalized status lookup result.
type StatusResponse struct {
	ProviderRequestID   string
	UserID              string
	Plan                string
	Status              string
	ExternalReferenceID string
	ActivatedAt         *time.Time
	TokenExpiresAt      *time.Time
	RawMessage          string
}

// Typed errors so callers can distinguish failure modes.
var (
	ErrTimeout      = errors.New("provider request timed out")
	ErrUnavailable  = errors.New("provider unavailable")
	ErrBadResponse  = errors.New("provider returned bad response")
	ErrNotFound     = errors.New("provider resource not found")
	ErrUnauthorized = errors.New("provider authentication failed")
)

// Registry is a simple name->Provider lookup. Built at startup and injected
// into the service layer.
type Registry struct {
	providers map[string]Provider
}

func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{providers: make(map[string]Provider, len(ps))}
	for _, p := range ps {
		r.providers[p.Name()] = p
	}
	return r
}

func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	return out
}
