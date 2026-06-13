// Package service contains the core business logic for the OTT integration:
// orchestrating partner calls, persisting activation state, and producing the
// SMS-style activation link surfaced to callers.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"backend/internal/model"
	"backend/internal/provider"
	"backend/internal/storage"
)

// Service-level errors (kept distinct from provider errors so handlers can
// decide HTTP status mapping cleanly).
var (
	ErrUnknownProvider = errors.New("unknown provider")
	ErrInvalidRequest  = errors.New("invalid request")
	ErrNotFound        = errors.New("subscription not found")
	ErrNotActivatable  = errors.New("subscription not in an activatable state")
)

// Clock allows tests to inject deterministic time.
type Clock func() time.Time

// CodeGenerator produces the 6-char activation code embedded in the SMS link.
type CodeGenerator func() (string, error)

// SubscriptionService orchestrates subscribe / activate / status flows across
// any registered provider.
type SubscriptionService struct {
	registry        *provider.Registry
	storage         *storage.MemoryStorage
	frontendBaseURL string
	now             Clock
	genCode         CodeGenerator
}

// Config bundles dependencies for NewSubscriptionService.
type Config struct {
	Registry        *provider.Registry
	Storage         *storage.MemoryStorage
	FrontendBaseURL string // e.g. https://frontend.local:5173
	Now             Clock
	GenCode         CodeGenerator
}

// NewSubscriptionService wires up a service with sane defaults for clock
// and code generator if not provided.
func NewSubscriptionService(cfg Config) *SubscriptionService {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.GenCode == nil {
		cfg.GenCode = generateActivationCode
	}
	return &SubscriptionService{
		registry:        cfg.Registry,
		storage:         cfg.Storage,
		frontendBaseURL: strings.TrimRight(cfg.FrontendBaseURL, "/"),
		now:             cfg.Now,
		genCode:         cfg.GenCode,
	}
}

// ---- Public DTOs (handler<->service contract) -------------------------------

type SubscribeInput struct {
	UserID   string
	MSISDN   string
	Provider string
	Plan     string
}

// SubscribeResult is the normalized result for a subscribe call, including
// the simulated SMS payload that the caller (post-purchase platform) would
// hand to the user.
type SubscribeResult struct {
	Subscription   model.Subscription
	ActivationLink string
	SMSMessage     string
}

// ---- Subscribe --------------------------------------------------------------

// Subscribe performs the post-purchase flow:
//  1. Look up the provider.
//  2. Generate an idempotency key and our local activation code.
//  3. Call the partner Subscribe API and normalize the response.
//  4. Persist a Subscription record keyed by activation code.
//  5. Build the activation link and SMS-style message returned to the caller.
func (s *SubscriptionService) Subscribe(ctx context.Context, in SubscribeInput) (*SubscribeResult, error) {
	if in.UserID == "" || in.MSISDN == "" || in.Plan == "" || in.Provider == "" {
		return nil, fmt.Errorf("%w: userId, msisdn, provider, plan are required", ErrInvalidRequest)
	}

	p, ok := s.registry.Get(in.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, in.Provider)
	}

	code, err := s.genCode()
	if err != nil {
		return nil, fmt.Errorf("generate activation code: %w", err)
	}

	resp, err := p.Subscribe(ctx, provider.SubscribeRequest{
		UserID:         in.UserID,
		MSISDN:         in.MSISDN,
		Plan:           in.Plan,
		IdempotencyKey: uuid.NewString(),
	})
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	status := resp.Status
	if status == "" || status == model.StatusUnknown {
		// Partner accepted the request but did not echo a status; treat as
		// pending activation by default.
		status = model.StatusPendingActivation
	}

	sub := &model.Subscription{
		SubscriptionRequestID: "REQ-" + strings.ToUpper(uuid.NewString()[:8]),
		ActivationCode:        code,
		UserID:                in.UserID,
		MSISDN:                in.MSISDN,
		Provider:              p.Name(),
		Plan:                  in.Plan,
		ActivationToken:       resp.ActivationToken,
		ProviderRequestID:     resp.ProviderRequestID,
		SubscriptionStatus:    status,
		CreatedAt:             now,
		UpdatedAt:             now,
		LastMessage:           resp.RawMessage,
	}
	if err := s.storage.Save(sub); err != nil {
		return nil, fmt.Errorf("persist subscription: %w", err)
	}

	link := s.buildActivationLink(code)
	return &SubscribeResult{
		Subscription:   *sub,
		ActivationLink: link,
		SMSMessage:     buildSMSMessage(p.Name(), link),
	}, nil
}

// ---- Activate ---------------------------------------------------------------

// Activate runs the activation step for a previously created subscription,
// identified by its activation code (from the SMS link).
func (s *SubscriptionService) Activate(ctx context.Context, code string) (*model.Subscription, error) {
	if code == "" {
		return nil, fmt.Errorf("%w: activation code required", ErrInvalidRequest)
	}
	sub, err := s.storage.GetByCode(code)
	if err != nil {
		return nil, ErrNotFound
	}

	// Allow re-activation only if not already active. If already active,
	// just return current state so the UI can render success idempotently.
	if sub.SubscriptionStatus == model.StatusActive {
		return sub, nil
	}
	if sub.ActivationToken == "" {
		return nil, ErrNotActivatable
	}

	p, ok := s.registry.Get(sub.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, sub.Provider)
	}

	resp, err := p.Activate(ctx, sub.ActivationToken)
	if err != nil {
		// Persist a non-terminal failure marker but still surface the error.
		sub.LastMessage = err.Error()
		sub.UpdatedAt = s.now().UTC()
		_ = s.storage.Save(sub)
		return nil, err
	}

	sub.SubscriptionStatus = resp.Status
	if resp.Plan != "" {
		sub.Plan = resp.Plan
	}
	if resp.ExternalReferenceID != "" {
		sub.ExternalReferenceID = resp.ExternalReferenceID
	}
	sub.ActivatedAt = resp.ActivatedAt
	sub.LastMessage = resp.RawMessage
	sub.UpdatedAt = s.now().UTC()

	if err := s.storage.Save(sub); err != nil {
		return nil, fmt.Errorf("persist activation: %w", err)
	}
	return sub, nil
}

// ---- Status -----------------------------------------------------------------

// GetStatusByCode returns the current local record, optionally refreshing
// from the provider if refresh=true and we have an activation token.
func (s *SubscriptionService) GetStatusByCode(ctx context.Context, code string, refresh bool) (*model.Subscription, error) {
	if code == "" {
		return nil, fmt.Errorf("%w: activation code required", ErrInvalidRequest)
	}
	sub, err := s.storage.GetByCode(code)
	if err != nil {
		return nil, ErrNotFound
	}
	if !refresh || sub.ActivationToken == "" {
		return sub, nil
	}

	p, ok := s.registry.Get(sub.Provider)
	if !ok {
		return sub, nil
	}
	resp, err := p.Status(ctx, sub.ActivationToken)
	if err != nil {
		// Refresh is best-effort; return last known state.
		return sub, nil
	}
	if resp.Status != "" && resp.Status != model.StatusUnknown {
		sub.SubscriptionStatus = resp.Status
	}
	if resp.ExternalReferenceID != "" {
		sub.ExternalReferenceID = resp.ExternalReferenceID
	}
	if resp.ActivatedAt != nil {
		sub.ActivatedAt = resp.ActivatedAt
	}
	if resp.TokenExpiresAt != nil {
		sub.TokenExpiresAt = resp.TokenExpiresAt
	}
	if resp.RawMessage != "" {
		sub.LastMessage = resp.RawMessage
	}
	sub.UpdatedAt = s.now().UTC()
	_ = s.storage.Save(sub)
	return sub, nil
}

// Providers returns the names of all registered providers (for /api/providers).
func (s *SubscriptionService) Providers() []string {
	return s.registry.Names()
}

// ---- Helpers ----------------------------------------------------------------

func (s *SubscriptionService) buildActivationLink(code string) string {
	base := s.frontendBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return base + "/activation/" + code
}

func buildSMSMessage(providerName, link string) string {
	return fmt.Sprintf(
		"Your %s Premium subscription is ready. Activate here: %s",
		providerName, link,
	)
}

// generateActivationCode returns a 6-character alphanumeric code suitable for
// embedding in a public URL. Uses crypto/rand for unpredictability.
func generateActivationCode() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	const n = 6
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
