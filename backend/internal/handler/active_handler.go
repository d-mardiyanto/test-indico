package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// activateRequest is the inbound payload from the activation page.
type activateRequest struct {
	ActivationCode string `json:"activationCode" binding:"required"`
}

// Activate handles POST /api/activate.
func (h *Handler) Activate(c *gin.Context) {
	var req activateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	sub, err := h.svc.Activate(c.Request.Context(), req.ActivationCode)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sub)
}

// SubscriptionStatus handles GET /api/subscription-status?activationCode=...
// (Optional refresh=true triggers a partner status refresh.)
func (h *Handler) SubscriptionStatus(c *gin.Context) {
	code := c.Query("activationCode")
	if code == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "activationCode query param required")
		return
	}
	refresh := c.Query("refresh") == "true"
	sub, err := h.svc.GetStatusByCode(c.Request.Context(), code, refresh)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, sub)
}
