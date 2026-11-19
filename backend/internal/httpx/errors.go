// Package httpx holds shared HTTP response helpers.
package httpx

import "github.com/gin-gonic/gin"

// Fail aborts a console API request with {"error": msg}.
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// OpenAIError aborts a /v1 request with the OpenAI error shape.
func OpenAIError(c *gin.Context, status int, typ, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": msg, "type": typ, "code": code}})
}
