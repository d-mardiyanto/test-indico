package handler

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

// ListProviders handles GET /api/providers. Useful for the frontend or ops
// teams to see which partners are wired in at runtime.
func (h *Handler) ListProviders(c *gin.Context) {
	names := h.svc.Providers()
	sort.Strings(names)
	c.JSON(http.StatusOK, gin.H{"providers": names})
}
