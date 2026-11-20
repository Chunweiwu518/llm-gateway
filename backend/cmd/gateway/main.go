package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/admin"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/apikey"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/auth"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingsvc"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/config"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/events"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/httpx"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/ratelimit"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/relay"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// commit is injected at build time with -ldflags "-X main.commit=<sha>".
var commit = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := store.Open(cfg.DSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := store.ConfigurePool(db, cfg.DBMaxOpen, cfg.DBMaxIdle, cfg.DBMaxLifetime); err != nil {
		log.Fatalf("db pool: %v", err)
	}
	if err := store.SeedAdmin(db, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	if err := store.SeedDefaults(db, cfg.MockUpstreamURL); err != nil {
		log.Fatalf("seed defaults: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), httpx.LimitBody(httpx.MaxBodyBytes))
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatalf("trusted proxies: %v", err)
	}
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "commit": commit})
	})
	r.StaticFile("/openapi.yaml", "docs/openapi.yaml")
	r.GET("/docs", func(c *gin.Context) { c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerHTML)) })

	limiter := ratelimit.New(rdb)
	authH := &auth.Handler{DB: db, JWT: auth.NewManager(cfg.JWTSecret), CookieSecure: cfg.CookieSecure, Limiter: limiter}
	requireUser := authH.RequireUser()
	api := r.Group("/api")
	authH.Routes(api)
	(&apikey.Handler{DB: db}).Routes(api, requireUser)
	(&billing.Handler{DB: db}).Routes(api, requireUser)
	(&admin.Handler{DB: db}).Routes(api.Group("/admin", requireUser, auth.RequireAdmin()))

	v1 := r.Group("/v1", apikey.AuthenticateWith(db, cfg.LastUsedEvery), ratelimit.Middleware(limiter))
	var pub *events.Publisher
	relayH := &relay.Handler{DB: db, Router: relay.NewRouter(db, cfg.UpstreamTimeout)}
	if cfg.BillingAddr != "" {
		bc, err := billingsvc.NewClient(cfg.BillingAddr)
		if err != nil {
			log.Fatalf("billing client: %v", err)
		}
		relayH.Settler = bc
		log.Printf("settling through billing service at %s", cfg.BillingAddr)
	}
	if len(cfg.KafkaBrokers) > 0 {
		pub = events.NewPublisher(cfg.KafkaBrokers, cfg.KafkaTopic, db)
		relayH.Usage = pub
		log.Printf("publishing usage events to kafka %v topic %s", cfg.KafkaBrokers, cfg.KafkaTopic)
	}
	relayH.Routes(v1)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("gateway %s listening on %s", commit, cfg.ListenAddr)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	// On SIGTERM finish in-flight requests, then flush queued Kafka records before exiting.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if pub != nil {
		if err := pub.Close(); err != nil {
			log.Printf("flush kafka: %v", err)
		}
	}
}

const swaggerHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>LLM Gateway API</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css"></head>
<body><div id="ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: "/openapi.yaml", dom_id: "#ui"});</script>
</body></html>`
