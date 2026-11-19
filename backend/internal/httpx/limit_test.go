package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLimitBodyRejectsLargeJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LimitBody(16))
	r.POST("/", func(c *gin.Context) {
		var v map[string]any
		if err := c.ShouldBindJSON(&v); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	})
	for body, want := range map[string]int{`{"a":"b"}`: 200, `{"a":"` + strings.Repeat("x", 64) + `"}`: 400} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("body len %d: want %d got %d", len(body), want, w.Code)
		}
	}
}
