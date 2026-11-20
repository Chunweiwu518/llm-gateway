// Command billing is the gRPC billing service (go-zero zrpc). It settles calls,
// consumes usage events from Kafka into MySQL, and publishes low-balance alerts to RabbitMQ.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingpb"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/billingsvc"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/config"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/events"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

var configFile = flag.String("f", "etc/billing.yaml", "zrpc config file")

// Config is the zrpc server config; everything else comes from environment variables.
type Config struct {
	zrpc.RpcServerConf
}

func main() {
	flag.Parse()
	var c Config
	conf.MustLoad(*configFile, &c)

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN is required")
	}
	db, err := store.Open(dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := store.ConfigurePool(db, 32, 32, 5*time.Minute); err != nil {
		log.Fatalf("db pool: %v", err)
	}

	srv := &billingsvc.Server{DB: db, Threshold: 500_000}
	if v := os.Getenv("LOW_BALANCE_THRESHOLD"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Fatalf("invalid LOW_BALANCE_THRESHOLD: %v", err)
		}
		srv.Threshold = n
	}
	if url := os.Getenv("RABBITMQ_URL"); url != "" {
		alerts := events.NewAlertPublisher(url)
		defer alerts.Close()
		srv.Alerts = alerts
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	if brokers := config.Brokers(os.Getenv("KAFKA_BROKERS")); len(brokers) > 0 {
		topic := os.Getenv("KAFKA_USAGE_TOPIC")
		if topic == "" {
			topic = events.DefaultUsageTopic
		}
		if err := events.EnsureTopic(ctx, brokers, topic, events.UsageTopicPartitions); err != nil {
			log.Fatalf("kafka topic: %v", err)
		}
		consumer := events.NewConsumer(db, brokers, topic, "billing-usage")
		go func() { consumer.Run(ctx); close(done) }()
		log.Println("consuming usage events from kafka")
	} else {
		close(done)
	}

	rpc := zrpc.MustNewServer(c.RpcServerConf, func(g *grpc.Server) { billingpb.RegisterBillingServer(g, srv) })
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		cancel()
		rpc.Stop()
	}()
	log.Printf("billing gRPC service listening on %s", c.ListenOn)
	rpc.Start()
	<-done // let the consumer flush its last batch
}
