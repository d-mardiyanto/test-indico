package model

import "time"

// SubscriptionStatus values normalized across providers.
const (
	StatusPendingActivation = "pending_activation"
	StatusActive            = "active"
	StatusFailed            = "failed"
	StatusExpired           = "expired"
	StatusUnknown           = "unknown"
)

// Subscription is the canonical record stored locally.
// It is provider-agnostic; provider-specific fields are kept in ProviderRef.
type Subscription struct {
	// SubscriptionRequestID is our internal id for this subscribe attempt.
	SubscriptionRequestID string `json:"subscriptionRequestId"`
	// ActivationCode is the 6-char code embedded in the SMS activation link.
	ActivationCode string `json:"activationCode"`

	UserID   string `json:"userId"`
	MSISDN   string `json:"msisdn"`
	Provider string `json:"provider"`
	Plan     string `json:"plan"`

	// ActivationToken returned by the provider (opaque to frontend).
	ActivationToken string `json:"-"`

	// ProviderRequestID is the provider-side request id (e.g. SUBREQ-xxxx).
	ProviderRequestID   string `json:"providerRequestId,omitempty"`
	ExternalReferenceID string `json:"externalReferenceId,omitempty"`

	SubscriptionStatus string     `json:"subscriptionStatus"`
	ActivatedAt        *time.Time `json:"activatedAt,omitempty"`
	TokenExpiresAt     *time.Time `json:"tokenExpiresAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// LastMessage carries the latest human-readable provider message.
	LastMessage string `json:"message,omitempty"`
}
