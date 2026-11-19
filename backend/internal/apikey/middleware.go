package apikey

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// FromContext returns the key and owner set by Authenticate.
func FromContext(c *gin.Context) (*store.APIKey, *store.User) {
	return c.MustGet("apikey").(*store.APIKey), c.MustGet("keyuser").(*store.User)
}

func unauthorized(c *gin.Context, msg string) {
	httpx.OpenAIError(c, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", msg)
}

// Authenticate checks "Authorization: Bearer sk-..." for /v1 routes.
func Authenticate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		plain, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || !strings.HasPrefix(plain, "sk-") {
			unauthorized(c, "missing or malformed API key")
			return
		}
		var k store.APIKey
		if err := db.Where("key_hash = ?", Hash(plain)).First(&k).Error; err != nil {
			unauthorized(c, "invalid API key")
			return
		}
		var u store.User
		if err := db.First(&u, k.UserID).Error; err != nil || k.Disabled || u.Disabled {
			unauthorized(c, "API key or account disabled")
			return
		}
		if err := db.Model(&k).UpdateColumn("last_used_at", time.Now()).Error; err != nil {
			log.Printf("record key last use failed: %v", err)
		}
		c.Set("apikey", &k)
		c.Set("keyuser", &u)
		c.Next()
	}
}
