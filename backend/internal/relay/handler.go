package relay

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/apikey"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Handler serves the OpenAI-compatible /v1 API.
type Handler struct {
	DB     *gorm.DB
	Router *Router
}

// Routes registers /v1 routes on a group that already authenticates the key.
func (h *Handler) Routes(v1 *gin.RouterGroup) {
	v1.POST("/chat/completions", h.chat)
	v1.GET("/models", h.models)
}

func badRequest(c *gin.Context, code, msg string) {
	httpx.OpenAIError(c, http.StatusBadRequest, "invalid_request_error", code, msg)
}

// MaxCompletionTokens is the largest max_tokens a request may ask for.
const MaxCompletionTokens = 65536

// estimate is the amount reserved before the call: prompt chars/4 plus max_tokens.
// req.MaxTokens must already be set (see chat).
func estimate(req ChatRequest, p store.ModelPrice) int64 {
	chars := 0
	for _, m := range req.Messages {
		chars += len([]rune(m.Content))
	}
	return billing.Cost(p, chars/4+1, *req.MaxTokens)
}

func (h *Handler) chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid_json", "request body must be OpenAI chat format with string message content")
		return
	}
	if req.Stream {
		badRequest(c, "stream_unsupported", "stream=true is not supported by this gateway")
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		badRequest(c, "invalid_request", "model and messages are required")
		return
	}
	if req.MaxTokens == nil {
		n := DefaultMaxTokens
		req.MaxTokens = &n // forwarded upstream so completion stays within the reservation
	}
	if *req.MaxTokens < 1 || *req.MaxTokens > MaxCompletionTokens {
		badRequest(c, "invalid_max_tokens", fmt.Sprintf("max_tokens must be between 1 and %d", MaxCompletionTokens))
		return
	}
	var price store.ModelPrice
	if err := h.DB.Where("model = ?", req.Model).First(&price).Error; err != nil {
		badRequest(c, "model_not_found", "model "+req.Model+" is not available")
		return
	}
	key, user := apikey.FromContext(c)
	reserved := estimate(req, price)
	if err := billing.Reserve(h.DB, user.ID, reserved); err != nil {
		if errors.Is(err, billing.ErrInsufficient) {
			httpx.OpenAIError(c, http.StatusPaymentRequired, "insufficient_quota", "insufficient_quota",
				"balance is too low for this request, please top up")
			return
		}
		log.Printf("reserve failed: %v", err)
		httpx.OpenAIError(c, http.StatusInternalServerError, "server_error", "billing_error", "billing unavailable")
		return
	}
	start := time.Now()
	resp, ch, failovers, err := h.Router.Dispatch(c.Request.Context(), req)
	entry := store.UsageLog{UserID: user.ID, APIKeyID: key.ID, Model: req.Model, FailoverCount: failovers}
	if ch.ID != 0 {
		entry.ChannelID, entry.ChannelName = &ch.ID, ch.Name
	}
	var actual int64
	if err == nil {
		actual = billing.Cost(price, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		entry.PromptTokens, entry.CompletionTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
	}
	charged, serr := billing.Settle(h.DB, user.ID, reserved, actual)
	if serr != nil {
		log.Printf("settle failed for user %d (reserved %d actual %d): %v", user.ID, reserved, actual, serr)
	} else if charged < actual {
		log.Printf("balance short for user %d: cost %d, charged %d", user.ID, actual, charged)
	}
	actual = charged
	entry.Cost, entry.LatencyMs = actual, time.Since(start).Milliseconds()
	entry.StatusCode = h.respond(c, resp, err)
	if lerr := h.DB.Create(&entry).Error; lerr != nil {
		log.Printf("write usage log failed: %v", lerr)
	}
}

// respond writes the result and returns the HTTP status sent.
func (h *Handler) respond(c *gin.Context, resp ChatResponse, err error) int {
	var ue *UpstreamError
	switch {
	case err == nil:
		c.JSON(http.StatusOK, resp)
		return http.StatusOK
	case errors.Is(err, ErrNoChannel):
		httpx.OpenAIError(c, http.StatusServiceUnavailable, "upstream_error", "no_channel", "no upstream channel serves this model")
		return http.StatusServiceUnavailable
	case errors.As(err, &ue) && !ue.Retryable:
		httpx.OpenAIError(c, ue.Status, "invalid_request_error", "upstream_rejected", "upstream rejected the request: "+ue.Body)
		return ue.Status
	default:
		log.Printf("all channels failed: %v", err)
		httpx.OpenAIError(c, http.StatusBadGateway, "upstream_error", "upstream_unavailable", "all upstream channels failed")
		return http.StatusBadGateway
	}
}

func (h *Handler) models(c *gin.Context) {
	var prices []store.ModelPrice
	var chans []store.Channel
	if h.DB.Find(&prices).Error != nil || h.DB.Where("enabled = ?", true).Find(&chans).Error != nil {
		httpx.OpenAIError(c, http.StatusInternalServerError, "server_error", "db_error", "could not list models")
		return
	}
	data := []gin.H{}
	for _, p := range prices {
		if slices.ContainsFunc(chans, func(ch store.Channel) bool { return slices.Contains(ch.Models, p.Model) }) {
			data = append(data, gin.H{"id": p.Model, "object": "model", "owned_by": "gateway"})
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
