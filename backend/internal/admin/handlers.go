// Package admin serves management APIs for users, channels, prices and usage.
package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// Handler serves /api/admin. The group must already require an admin.
type Handler struct {
	DB *gorm.DB
}

// Routes registers admin routes on an admin-only group.
func (h *Handler) Routes(g *gin.RouterGroup) {
	g.GET("/users", h.listUsers)
	g.PATCH("/users/:id", h.updateUser)
	g.POST("/users/:id/credit", h.credit)
	g.GET("/channels", h.listChannels)
	g.POST("/channels", h.createChannel)
	g.PATCH("/channels/:id", h.updateChannel)
	g.DELETE("/channels/:id", h.deleteChannel)
	g.GET("/prices", h.listPrices)
	g.PUT("/prices/:model", h.putPrice)
	g.DELETE("/prices/:model", h.deletePrice)
	g.GET("/usage", h.usage)
}

func paramID(c *gin.Context) uint {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id)
}

func me(c *gin.Context) *store.User { return c.MustGet("user").(*store.User) }

func (h *Handler) listUsers(c *gin.Context) {
	users := []store.User{}
	if err := h.DB.Order("id").Find(&users).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list users")
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) updateUser(c *gin.Context) {
	var b struct {
		Role     *string `json:"role" binding:"omitempty,oneof=user admin"`
		Disabled *bool   `json:"disabled"`
	}
	if c.ShouldBindJSON(&b) != nil {
		httpx.Fail(c, http.StatusBadRequest, "role must be user or admin")
		return
	}
	id := paramID(c)
	if id == me(c).ID && ((b.Role != nil && *b.Role != store.RoleAdmin) || (b.Disabled != nil && *b.Disabled)) {
		httpx.Fail(c, http.StatusBadRequest, "you cannot demote or disable your own account")
		return
	}
	var u store.User
	if err := h.DB.First(&u, id).Error; err != nil {
		httpx.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	updates := map[string]any{}
	if b.Role != nil {
		updates["role"] = *b.Role
	}
	if b.Disabled != nil {
		updates["disabled"] = *b.Disabled
	}
	if err := h.DB.Model(&u).Updates(updates).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not change user")
		return
	}
	c.JSON(http.StatusOK, u)
}

func (h *Handler) credit(c *gin.Context) {
	var b struct {
		AmountUSD float64 `json:"amount_usd" binding:"required"`
	}
	if c.ShouldBindJSON(&b) != nil || b.AmountUSD < -billing.MaxTopupUSD*10 || b.AmountUSD > billing.MaxTopupUSD*10 {
		httpx.Fail(c, http.StatusBadRequest, "amount_usd must be a non-zero number within ±10000")
		return
	}
	err := billing.Topup(h.DB, paramID(c), billing.ToMicro(b.AmountUSD), store.TopupAdmin)
	if errors.Is(err, billing.ErrInsufficient) {
		httpx.Fail(c, http.StatusBadRequest, "user not found or balance would become negative")
		return
	}
	if err != nil {
		log.Printf("admin credit failed: %v", err)
		httpx.Fail(c, http.StatusInternalServerError, "credit failed")
		return
	}
	var u store.User
	h.DB.First(&u, paramID(c))
	c.JSON(http.StatusOK, u)
}

func (h *Handler) usage(c *gin.Context) {
	uid, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	billing.UsageQuery(h.DB, c, uint(uid))
}

func (h *Handler) listPrices(c *gin.Context) {
	prices := []store.ModelPrice{}
	if err := h.DB.Order("model").Find(&prices).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list prices")
		return
	}
	c.JSON(http.StatusOK, prices)
}

func (h *Handler) putPrice(c *gin.Context) {
	var b struct {
		InputPerM  *int64 `json:"input_per_m" binding:"required,min=0"`
		OutputPerM *int64 `json:"output_per_m" binding:"required,min=0"`
	}
	model := c.Param("model")
	if c.ShouldBindJSON(&b) != nil || len(model) > 100 {
		httpx.Fail(c, http.StatusBadRequest, "input_per_m and output_per_m must be non-negative integers (micro-USD per 1M tokens)")
		return
	}
	p := store.ModelPrice{Model: model, InputPerM: *b.InputPerM, OutputPerM: *b.OutputPerM}
	if err := h.DB.Save(&p).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not save price")
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) deletePrice(c *gin.Context) {
	if err := h.DB.Delete(&store.ModelPrice{}, "model = ?", c.Param("model")).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not delete price")
		return
	}
	c.Status(http.StatusNoContent)
}

// channelView adds a masked key to the channel JSON.
type channelView struct {
	store.Channel
	APIKeyMasked string `json:"api_key_masked"`
}

func view(ch store.Channel) channelView {
	masked := "****"
	if len(ch.APIKey) >= 8 {
		masked += ch.APIKey[len(ch.APIKey)-4:]
	}
	return channelView{Channel: ch, APIKeyMasked: masked}
}

func (h *Handler) listChannels(c *gin.Context) {
	var chans []store.Channel
	if err := h.DB.Order("priority, id").Find(&chans).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not list channels")
		return
	}
	out := make([]channelView, 0, len(chans))
	for _, ch := range chans {
		out = append(out, view(ch))
	}
	c.JSON(http.StatusOK, out)
}

type channelInput struct {
	Name     *string           `json:"name" binding:"omitempty,min=1,max=100"`
	Type     *string           `json:"type" binding:"omitempty,oneof=openai anthropic"`
	BaseURL  *string           `json:"base_url" binding:"omitempty,url,max=255"`
	APIKey   *string           `json:"api_key" binding:"omitempty,max=255"`
	Models   []string          `json:"models"`
	ModelMap map[string]string `json:"model_map"`
	Priority *int              `json:"priority" binding:"omitempty,min=0,max=1000"`
	Enabled  *bool             `json:"enabled"`
}

// apply copies the provided fields onto ch. A blank api_key keeps the old one.
func (in channelInput) apply(ch *store.Channel) {
	setIf(&ch.Name, in.Name)
	setIf(&ch.Type, in.Type)
	setIf(&ch.BaseURL, in.BaseURL)
	setIf(&ch.Priority, in.Priority)
	setIf(&ch.Enabled, in.Enabled)
	if in.APIKey != nil && *in.APIKey != "" {
		ch.APIKey = *in.APIKey
	}
	if in.Models != nil {
		ch.Models = datatypes.JSONSlice[string](in.Models)
	}
	if in.ModelMap != nil {
		ch.ModelMap = datatypes.NewJSONType(in.ModelMap)
	}
}

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

const channelHelp = "name, type (openai|anthropic), base_url (URL) and api_key are required"

func (h *Handler) createChannel(c *gin.Context) {
	var in channelInput
	if c.ShouldBindJSON(&in) != nil || in.Name == nil || in.Type == nil || in.BaseURL == nil || in.APIKey == nil || *in.APIKey == "" {
		httpx.Fail(c, http.StatusBadRequest, channelHelp)
		return
	}
	ch := store.Channel{Priority: 1, Enabled: true, Models: datatypes.JSONSlice[string]{},
		ModelMap: datatypes.NewJSONType(map[string]string{})}
	in.apply(&ch)
	if err := h.DB.Create(&ch).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not create channel")
		return
	}
	c.JSON(http.StatusCreated, view(ch))
}

func (h *Handler) updateChannel(c *gin.Context) {
	var in channelInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, http.StatusBadRequest, channelHelp)
		return
	}
	var ch store.Channel
	if err := h.DB.First(&ch, paramID(c)).Error; err != nil {
		httpx.Fail(c, http.StatusNotFound, "channel not found")
		return
	}
	in.apply(&ch)
	if err := h.DB.Save(&ch).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not save channel")
		return
	}
	c.JSON(http.StatusOK, view(ch))
}

func (h *Handler) deleteChannel(c *gin.Context) {
	if err := h.DB.Delete(&store.Channel{}, paramID(c)).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, "could not delete channel")
		return
	}
	c.Status(http.StatusNoContent)
}
