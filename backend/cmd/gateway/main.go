package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/config"
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
	if err := store.SeedAdmin(db, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "commit": commit})
	})
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: r, ReadHeaderTimeout: 10e9}
	log.Fatal(srv.ListenAndServe())
}
