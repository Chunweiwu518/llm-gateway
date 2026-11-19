// Package httpx holds shared HTTP response helpers.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Fail aborts a console API request with {"error": msg}.
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// OpenAIError aborts a /v1 request with the OpenAI error shape.
func OpenAIError(c *gin.Context, status int, typ, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": msg, "type": typ, "code": code}})
}

// MaxBodyBytes caps request bodies for every route.
const MaxBodyBytes = 1 << 20

// LimitBody stops reading request bodies after n bytes, so oversized JSON fails to bind.
func LimitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
