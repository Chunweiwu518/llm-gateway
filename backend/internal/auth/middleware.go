package auth

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Cookie names.
const (
	SessionCookie = "session"
	PendingCookie = "pending"
)

// CurrentUser returns the user set by RequireUser.
func CurrentUser(c *gin.Context) *store.User {
	return c.MustGet("user").(*store.User)
}

// RequireUser accepts only a full-stage session for an active user.
func (h *Handler) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, err := c.Cookie(SessionCookie)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, "not logged in")
			return
		}
		claims, err := h.JWT.Parse(tok)
		if err != nil || claims.Stage != StageFull {
			httpx.Fail(c, http.StatusUnauthorized, "invalid session")
			return
		}
		id, err := strconv.ParseUint(claims.Subject, 10, 64)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, "invalid session")
			return
		}
		var u store.User
		if err := h.DB.First(&u, id).Error; err != nil || u.Disabled {
			httpx.Fail(c, http.StatusUnauthorized, "account unavailable")
			return
		}
		c.Set("user", &u)
		c.Next()
	}
}

// RequireAdmin must run after RequireUser.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if CurrentUser(c).Role != store.RoleAdmin {
			httpx.Fail(c, http.StatusForbidden, "admin only")
			return
		}
		c.Next()
	}
}
