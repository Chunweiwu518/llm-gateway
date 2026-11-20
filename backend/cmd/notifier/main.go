// Command notifier consumes low-balance alerts from RabbitMQ and logs them.
// A real deployment would send an email or chat message here.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/events"
)

func main() {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("notifier waiting for alerts on queue %s", events.LowBalanceQueue)
	events.ConsumeLowBalance(ctx, url, func(m events.LowBalance) {
		log.Printf("LOW BALANCE ALERT user=%d balance=%.4f USD threshold=%.4f USD at=%s",
			m.UserID, float64(m.Balance)/1e6, float64(m.Threshold)/1e6, m.At.Format("2006-01-02T15:04:05Z"))
	})
}
