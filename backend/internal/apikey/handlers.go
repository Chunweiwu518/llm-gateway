package apikey

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Handler serves /api/keys for the logged-in user.
type Handler struct {
	DB *gorm.DB
}

func currentUser(c *gin.Context) *store.User { return c.MustGet("user").(*store.User) }

// Routes registers key routes; requireUser must set "user" in the context.
func (h *Handler) Routes(api *gin.RouterGroup, requireUser gin.HandlerFunc) {
	g := api.Group("/keys", requireUser)
	g.GET("", h.list)
	g.POST("", h.create)
	g.PATCH("/:id", h.setDisabled)
}

func (h *Handler) list(c *gin.Context) {
	keys := []store.APIKey{}
	if err := h.DB.Where("user_id = ?", currentUser(c).ID).Order("id desc").Find(&keys).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list keys")
		return
	}
	c.JSON(http.StatusOK, keys)
}

func (h *Handler) create(c *gin.Context) {
	var b struct {
		Name string `json:"name" binding:"required,max=100"`
	}
	if c.ShouldBindJSON(&b) != nil {
		httpx.Fail(c, http.StatusBadRequest, "name required (max 100 characters)")
		return
	}
	plain, hash, prefix, err := Generate()
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not generate key")
		return
	}
	k := store.APIKey{UserID: currentUser(c).ID, Name: b.Name, KeyHash: hash, KeyPrefix: prefix, RPMLimit: 60}
	if err := h.DB.Create(&k).Error; err != nil {
		log.Printf("create key failed: %v", err)
		httpx.Fail(c, http.StatusInternalServerError, "could not save key")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": k.ID, "name": k.Name, "key": plain, "key_prefix": prefix, "rpm_limit": k.RPMLimit})
}

func (h *Handler) setDisabled(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var b struct {
		Disabled *bool `json:"disabled" binding:"required"`
	}
	if c.ShouldBindJSON(&b) != nil {
		httpx.Fail(c, http.StatusBadRequest, "disabled (bool) required")
		return
	}
	var k store.APIKey
	if err := h.DB.Where("id = ? AND user_id = ?", id, currentUser(c).ID).First(&k).Error; err != nil {
		httpx.Fail(c, http.StatusNotFound, "key not found")
		return
	}
	if err := h.DB.Model(&k).Update("disabled", *b.Disabled).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not change key")
		return
	}
	c.JSON(http.StatusOK, k)
}
