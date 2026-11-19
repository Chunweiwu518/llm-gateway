package auth

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/ratelimit"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Attempt limits per minute. Every attempt counts, successful or not.
const (
	LoginAttemptsPerMinute = 10 // per email
	TOTPAttemptsPerMinute  = 5  // per user
)

// Handler serves /api/auth and /api/me.
type Handler struct {
	DB           *gorm.DB
	JWT          *Manager
	CookieSecure bool
	Limiter      *ratelimit.Limiter // nil disables attempt limits
}

// tooMany counts an attempt and answers 429 when the limit is exceeded.
func (h *Handler) tooMany(c *gin.Context, key string, limit int) bool {
	if h.Limiter == nil {
		return false
	}
	ok, err := h.Limiter.AllowKey(c.Request.Context(), key, limit)
	if err != nil {
		log.Printf("attempt limiter unavailable, allowing request: %v", err)
	}
	if !ok {
		httpx.Fail(c, http.StatusTooManyRequests, "too many attempts, try again in a minute")
	}
	return !ok
}

// dummyHash keeps login timing similar for unknown emails.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

type credentials struct {
	Email    string `json:"email" binding:"required,email,max=191"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type codeBody struct {
	Code string `json:"code" binding:"required,len=6,numeric"`
}

// Routes registers auth routes under /api.
func (h *Handler) Routes(api *gin.RouterGroup) {
	api.POST("/auth/register", h.register)
	api.POST("/auth/login", h.login)
	api.POST("/auth/login/totp", h.loginTOTP)
	api.POST("/auth/logout", h.logout)
	me := api.Group("/me", h.RequireUser())
	me.GET("", func(c *gin.Context) { c.JSON(http.StatusOK, CurrentUser(c)) })
	me.POST("/totp/setup", h.totpSetup)
	me.POST("/totp/enable", h.totpEnable)
	me.POST("/totp/disable", h.totpDisable)
}

func (h *Handler) setCookie(c *gin.Context, name, value string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, int(ttl.Seconds()), "/api", "", h.CookieSecure, true)
}

func (h *Handler) startSession(c *gin.Context, u *store.User) {
	tok, err := h.JWT.Issue(u.ID, u.Role, StageFull, SessionTTL)
	if err != nil {
		log.Printf("issue session failed: %v", err)
		httpx.Fail(c, http.StatusInternalServerError, "could not start session")
		return
	}
	h.setCookie(c, PendingCookie, "", -time.Second)
	h.setCookie(c, SessionCookie, tok, SessionTTL)
	c.JSON(http.StatusOK, gin.H{"user": u})
}

func (h *Handler) register(c *gin.Context) {
	var b credentials
	if err := c.ShouldBindJSON(&b); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "valid email and password of 8-72 characters required")
		return
	}
	hash, err := store.HashPassword(b.Password)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not hash password")
		return
	}
	u := store.User{Email: strings.ToLower(b.Email), PasswordHash: hash, Role: store.RoleUser}
	if err := h.DB.Create(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "Duplicate entry") {
			httpx.Fail(c, http.StatusConflict, "email already registered")
			return
		}
		log.Printf("create user failed: %v", err)
		httpx.Fail(c, http.StatusInternalServerError, "could not create user")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": u})
}

func (h *Handler) login(c *gin.Context) {
	var b credentials
	if err := c.ShouldBindJSON(&b); err != nil {
		httpx.Fail(c, http.StatusUnauthorized, "invalid email or password")
		return
	}
	email := strings.ToLower(b.Email)
	if h.tooMany(c, "login:"+email, LoginAttemptsPerMinute) {
		return
	}
	var u store.User
	found := h.DB.Where("email = ?", email).First(&u).Error == nil
	hash := dummyHash
	if found {
		hash = []byte(u.PasswordHash)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(b.Password)) != nil || !found {
		httpx.Fail(c, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if u.Disabled {
		httpx.Fail(c, http.StatusForbidden, "account disabled")
		return
	}
	if !u.TOTPEnabled {
		h.startSession(c, &u)
		return
	}
	tok, err := h.JWT.Issue(u.ID, u.Role, StagePendingTOTP, PendingTTL)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not start login")
		return
	}
	h.setCookie(c, PendingCookie, tok, PendingTTL)
	c.JSON(http.StatusOK, gin.H{"totp_required": true})
}

func (h *Handler) loginTOTP(c *gin.Context) {
	var b codeBody
	tok, cerr := c.Cookie(PendingCookie)
	if cerr != nil || c.ShouldBindJSON(&b) != nil {
		httpx.Fail(c, http.StatusUnauthorized, "invalid code")
		return
	}
	claims, err := h.JWT.Parse(tok)
	if err != nil || claims.Stage != StagePendingTOTP {
		httpx.Fail(c, http.StatusUnauthorized, "login expired, sign in again")
		return
	}
	if h.tooMany(c, "totp:"+claims.Subject, TOTPAttemptsPerMinute) {
		return
	}
	id, _ := strconv.ParseUint(claims.Subject, 10, 64)
	var u store.User
	if err := h.DB.First(&u, id).Error; err != nil || u.Disabled || u.TOTPSecret == nil {
		httpx.Fail(c, http.StatusUnauthorized, "invalid code")
		return
	}
	if !ValidTOTP(b.Code, *u.TOTPSecret) {
		httpx.Fail(c, http.StatusUnauthorized, "invalid code")
		return
	}
	h.startSession(c, &u)
}

func (h *Handler) logout(c *gin.Context) {
	h.setCookie(c, SessionCookie, "", -time.Second)
	h.setCookie(c, PendingCookie, "", -time.Second)
	c.Status(http.StatusNoContent)
}

func (h *Handler) totpSetup(c *gin.Context) {
	u := CurrentUser(c)
	if u.TOTPEnabled {
		httpx.Fail(c, http.StatusConflict, "two-factor already enabled")
		return
	}
	key, err := NewTOTPKey(u.Email)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not create secret")
		return
	}
	secret := key.Secret()
	if err := h.DB.Model(u).Update("totp_secret", secret).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not save secret")
		return
	}
	c.JSON(http.StatusOK, gin.H{"otpauth_url": key.URL(), "secret": secret})
}

func (h *Handler) setTOTP(c *gin.Context, enable bool) {
	u := CurrentUser(c)
	var b codeBody
	if c.ShouldBindJSON(&b) != nil || u.TOTPSecret == nil || u.TOTPEnabled == enable || !ValidTOTP(b.Code, *u.TOTPSecret) {
		httpx.Fail(c, http.StatusBadRequest, "invalid code")
		return
	}
	updates := map[string]any{"totp_enabled": enable}
	if !enable {
		updates["totp_secret"] = nil
	}
	if err := h.DB.Model(u).Updates(updates).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not update two-factor")
		return
	}
	c.JSON(http.StatusOK, gin.H{"totp_enabled": enable})
}

func (h *Handler) totpEnable(c *gin.Context)  { h.setTOTP(c, true) }
func (h *Handler) totpDisable(c *gin.Context) { h.setTOTP(c, false) }
