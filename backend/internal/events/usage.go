// Package events moves usage records through Kafka and balance alerts through RabbitMQ.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// DefaultUsageTopic is the Kafka topic that carries one store.UsageLog per call.
const DefaultUsageTopic = "usage.events"

// Publisher queues usage records to Kafka without blocking the request.
// When Kafka cannot take a record it writes the row to MySQL directly, so
// the call is still recorded.
type Publisher struct {
	w  *kafka.Writer
	db *gorm.DB
}

// NewPublisher returns a publisher that batches records (up to 100 or 20 ms).
func NewPublisher(brokers []string, topic string, db *gorm.DB) *Publisher {
	p := &Publisher{db: db}
	p.w = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		BatchSize:              100,
		BatchTimeout:           20 * time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		Async:                  true,
		AllowAutoTopicCreation: true,
		Completion:             p.completed,
	}
	return p
}

// Record queues e. It never returns an error: failures fall back to MySQL.
func (p *Publisher) Record(e store.UsageLog) {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	val, err := json.Marshal(e)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err = p.w.WriteMessages(ctx, kafka.Message{Key: []byte(strconv.FormatUint(uint64(e.UserID), 10)), Value: val})
	}
	if err != nil {
		log.Printf("kafka publish failed, writing usage row directly: %v", err)
		p.insert([]store.UsageLog{e})
	}
}

// completed runs after kafka-go delivers (or gives up on) a batch.
func (p *Publisher) completed(msgs []kafka.Message, err error) {
	if err == nil {
		return
	}
	log.Printf("kafka delivery failed for %d usage records, writing directly: %v", len(msgs), err)
	rows := decode(msgs)
	p.insert(rows)
}

func (p *Publisher) insert(rows []store.UsageLog) {
	if len(rows) == 0 {
		return
	}
	if err := p.db.CreateInBatches(rows, 200).Error; err != nil {
		log.Printf("write usage log failed: %v", err)
	}
}

// Close flushes pending records.
func (p *Publisher) Close() error { return p.w.Close() }

func decode(msgs []kafka.Message) []store.UsageLog {
	rows := make([]store.UsageLog, 0, len(msgs))
	for _, m := range msgs {
		var e store.UsageLog
		if err := json.Unmarshal(m.Value, &e); err != nil {
			log.Printf("skipping undecodable usage event at offset %d: %v", m.Offset, err)
			continue
		}
		e.ID = 0
		rows = append(rows, e)
	}
	return rows
}

// Consumer reads usage events and inserts them into MySQL in batches.
// Delivery is at-least-once: a crash between the insert and the offset commit
// can write a few rows twice.
type Consumer struct {
	DB       *gorm.DB
	MaxBatch int
	MaxWait  time.Duration
	r        *kafka.Reader
}

// NewConsumer joins consumer group `group` on topic.
func NewConsumer(db *gorm.DB, brokers []string, topic, group string) *Consumer {
	return &Consumer{
		DB: db, MaxBatch: 500, MaxWait: 200 * time.Millisecond,
		r: kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers, Topic: topic, GroupID: group,
			MinBytes: 1, MaxBytes: 10 << 20, MaxWait: 100 * time.Millisecond,
			WatchPartitionChanges: true,
		}),
	}
}

// Run consumes until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	defer func() {
		if err := c.r.Close(); err != nil {
			log.Printf("close usage reader: %v", err)
		}
	}()
	var pending []kafka.Message
	for {
		fctx, cancel := context.WithTimeout(ctx, c.MaxWait)
		m, err := c.r.FetchMessage(fctx)
		cancel()
		if err == nil {
			pending = append(pending, m)
		} else if ctx.Err() != nil {
			if len(pending) > 0 {
				c.flush(context.Background(), pending)
			}
			return
		} else if !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("usage fetch failed: %v", err)
			time.Sleep(time.Second)
		}
		if len(pending) >= c.MaxBatch || (err != nil && len(pending) > 0) {
			if c.flush(ctx, pending) {
				pending = pending[:0]
			} else {
				time.Sleep(time.Second)
			}
		}
	}
}

// flush inserts msgs in one batch and commits their offsets. It reports success.
func (c *Consumer) flush(ctx context.Context, msgs []kafka.Message) bool {
	if rows := decode(msgs); len(rows) > 0 {
		if err := c.DB.WithContext(ctx).CreateInBatches(rows, 500).Error; err != nil {
			log.Printf("batch insert of %d usage rows failed, will retry: %v", len(rows), err)
			return false
		}
	}
	if err := c.r.CommitMessages(ctx, msgs...); err != nil {
		log.Printf("commit usage offsets failed: %v", err)
		return false
	}
	return true
}
