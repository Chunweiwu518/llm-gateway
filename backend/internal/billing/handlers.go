package billing

import (
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Top-up limits per request, in USD.
const (
	MinTopupUSD = 1
	MaxTopupUSD = 1000
	usagePage   = 50
)

// Handler serves balance, top-up and usage for the logged-in user.
type Handler struct {
	DB *gorm.DB
}

func currentUser(c *gin.Context) *store.User { return c.MustGet("user").(*store.User) }

// ToMicro converts dollars to micro-USD.
func ToMicro(usd float64) int64 { return int64(math.Round(usd * float64(store.MicroPerUSD))) }

// Routes registers billing routes; requireUser must set "user".
func (h *Handler) Routes(api *gin.RouterGroup, requireUser gin.HandlerFunc) {
	api.GET("/billing/balance", requireUser, h.balance)
	api.POST("/billing/topup", requireUser, h.topup)
	api.GET("/billing/topups", requireUser, h.topups)
	api.GET("/usage", requireUser, h.usage)
	api.GET("/usage/daily", requireUser, h.daily)
}

func (h *Handler) balance(c *gin.Context) {
	var u store.User
	if err := h.DB.First(&u, currentUser(c).ID).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not read balance")
		return
	}
	c.JSON(http.StatusOK, gin.H{"balance": u.Balance})
}

func (h *Handler) topup(c *gin.Context) {
	var b struct {
		AmountUSD float64 `json:"amount_usd"`
	}
	if c.ShouldBindJSON(&b) != nil || b.AmountUSD < MinTopupUSD || b.AmountUSD > MaxTopupUSD {
		httpx.Fail(c, http.StatusBadRequest, "amount_usd must be between 1 and 1000")
		return
	}
	u := currentUser(c)
	if err := Topup(h.DB, u.ID, ToMicro(b.AmountUSD), store.TopupSelf); err != nil {
		log.Printf("topup for user %d failed: %v", u.ID, err)
		httpx.Fail(c, http.StatusInternalServerError, "top-up failed")
		return
	}
	h.balance(c)
}

func (h *Handler) topups(c *gin.Context) {
	list := []store.Topup{}
	if err := h.DB.Where("user_id = ?", currentUser(c).ID).Order("id desc").Limit(100).Find(&list).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list top-ups")
		return
	}
	c.JSON(http.StatusOK, list)
}

// UsageQuery lists usage logs, optionally for one user (userID 0 = everyone).
func UsageQuery(db *gorm.DB, c *gin.Context, userID uint) {
	q := db.Model(&store.UsageLog{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if from, err := time.Parse(time.DateOnly, c.Query("from")); err == nil {
		q = q.Where("created_at >= ?", from)
	}
	if to, err := time.Parse(time.DateOnly, c.Query("to")); err == nil {
		q = q.Where("created_at < ?", to.AddDate(0, 0, 1))
	}
	var sum struct{ Count, Prompt, Completion, Cost int64 }
	if err := q.Session(&gorm.Session{}).Select("COUNT(*) AS count, COALESCE(SUM(prompt_tokens),0) AS prompt, " +
		"COALESCE(SUM(completion_tokens),0) AS completion, COALESCE(SUM(cost),0) AS cost").Scan(&sum).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not sum usage")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	page = max(page, 1)
	items := []store.UsageLog{}
	if err := q.Order("id desc").Offset((page - 1) * usagePage).Limit(usagePage).Find(&items).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list usage")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "page_size": usagePage, "count": sum.Count,
		"prompt_tokens": sum.Prompt, "completion_tokens": sum.Completion, "cost": sum.Cost})
}

func (h *Handler) usage(c *gin.Context) { UsageQuery(h.DB, c, currentUser(c).ID) }

func (h *Handler) daily(c *gin.Context) {
	type row struct {
		Day      string `json:"day"`
		Requests int64  `json:"requests"`
		Tokens   int64  `json:"tokens"`
		Cost     int64  `json:"cost"`
	}
	rows := []row{}
	since := time.Now().UTC().AddDate(0, 0, -6).Truncate(24 * time.Hour)
	err := h.DB.Model(&store.UsageLog{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS day, COUNT(*) AS requests, "+
			"SUM(prompt_tokens + completion_tokens) AS tokens, SUM(cost) AS cost").
		Where("user_id = ? AND created_at >= ?", currentUser(c).ID, since).
		Group("day").Order("day").Scan(&rows).Error
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not summarize usage")
		return
	}
	c.JSON(http.StatusOK, rows)
}
