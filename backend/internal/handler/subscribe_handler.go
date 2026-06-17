// Package handler exposes HTTP entry points (Gin handlers) that translate
// HTTP requests into service calls and service results into JSON responses.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend/internal/model"
	"backend/internal/provider"
	"backend/internal/service"
)

// Handler bundles dependencies shared by all HTTP handlers.
type Handler struct {
	svc *service.SubscriptionService
}

// New constructs a Handler bound to the given service.
func New(svc *service.SubscriptionService) *Handler {
	return &Handler{svc: svc}
}

// Register attaches all routes under /api to the given router.
func (h *Handler) Register(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/subscribe", h.Subscribe)
		api.POST("/activate", h.Activate)
		api.GET("/subscription-status", h.SubscriptionStatus)
		api.GET("/providers", h.ListProviders)
	}
}

// ---- /api/subscribe ---------------------------------------------------------

// subscribeRequest is the inbound payload from the post-purchase platform.
// Standard fields (userId, msisdn, plan) are used by NETPLAY and NETFLIX.
// Provider-specific fields (accountEmail, subscriptionTier, region, profileName)
// are passed through as Extras for providers like DISNEYPLUS.
type subscribeRequest struct {
	Provider string `json:"provider" binding:"required"`
	// NETPLAY / NETFLIX
	UserID string `json:"userId"`
	MSISDN string `json:"msisdn"`
	Plan   string `json:"plan"`
	// DISNEYPLUS
	AccountEmail     string `json:"accountEmail"`
	SubscriptionTier string `json:"subscriptionTier"`
	Region           string `json:"region"`
	ProfileName      string `json:"profileName"`
}

// subscribeResponse mirrors the SMS-style payload returned to the caller.
type subscribeResponse struct {
	SubscriptionRequestID string             `json:"subscriptionRequestId"`
	ActivationCode        string             `json:"activationCode"`
	ActivationLink        string             `json:"activationLink"`
	SMSMessage            string             `json:"smsMessage"`
	Status                string             `json:"status"`
	Subscription          model.Subscription `json:"subscription"`
}

// Subscribe handles POST /api/subscribe.
func (h *Handler) Subscribe(c *gin.Context) {
	var req subscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	extras := map[string]string{}
	if req.AccountEmail != "" {
		extras["accountEmail"] = req.AccountEmail
	}
	if req.SubscriptionTier != "" {
		extras["subscriptionTier"] = req.SubscriptionTier
	}
	if req.Region != "" {
		extras["region"] = req.Region
	}
	if req.ProfileName != "" {
		extras["profileName"] = req.ProfileName
	}

	res, err := h.svc.Subscribe(c.Request.Context(), service.SubscribeInput{
		UserID:   req.UserID,
		MSISDN:   req.MSISDN,
		Provider: req.Provider,
		Plan:     req.Plan,
		Extras:   extras,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, subscribeResponse{
		SubscriptionRequestID: res.Subscription.SubscriptionRequestID,
		ActivationCode:        res.Subscription.ActivationCode,
		ActivationLink:        res.ActivationLink,
		SMSMessage:            res.SMSMessage,
		Status:                res.Subscription.SubscriptionStatus,
		Subscription:          res.Subscription,
	})
}

// ---- shared error helpers ---------------------------------------------------

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeError(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, errorBody{Error: code, Message: msg})
}

// writeServiceError maps service- and provider-layer errors to HTTP status.
func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRequest):
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, service.ErrUnknownProvider):
		writeError(c, http.StatusBadRequest, "unknown_provider", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, service.ErrNotActivatable):
		writeError(c, http.StatusConflict, "not_activatable", err.Error())
	case errors.Is(err, provider.ErrTimeout):
		writeError(c, http.StatusGatewayTimeout, "provider_timeout", err.Error())
	case errors.Is(err, provider.ErrUnauthorized):
		writeError(c, http.StatusBadGateway, "provider_unauthorized", err.Error())
	case errors.Is(err, provider.ErrNotFound):
		writeError(c, http.StatusNotFound, "provider_not_found", err.Error())
	case errors.Is(err, provider.ErrUnavailable):
		writeError(c, http.StatusBadGateway, "provider_unavailable", err.Error())
	case errors.Is(err, provider.ErrBadResponse):
		writeError(c, http.StatusBadGateway, "provider_bad_response", err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
