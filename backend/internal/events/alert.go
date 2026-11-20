package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// LowBalanceQueue is the RabbitMQ queue for low-balance notifications.
const LowBalanceQueue = "notifications.low_balance"

// LowBalance tells a user (via whatever consumes the queue) that the balance is low.
// Amounts are micro-USD.
type LowBalance struct {
	UserID    uint      `json:"user_id"`
	Balance   int64     `json:"balance"`
	Threshold int64     `json:"threshold"`
	At        time.Time `json:"at"`
}

// AlertPublisher publishes to RabbitMQ and connects lazily, so the billing
// service starts even when the broker is down.
type AlertPublisher struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewAlertPublisher returns a publisher for the amqp:// URL.
func NewAlertPublisher(url string) *AlertPublisher { return &AlertPublisher{url: url} }

func declare(ch *amqp.Channel) error {
	_, err := ch.QueueDeclare(LowBalanceQueue, true, false, false, false, nil)
	return err
}

func (a *AlertPublisher) channel() (*amqp.Channel, error) {
	if a.ch != nil && !a.ch.IsClosed() {
		return a.ch, nil
	}
	conn, err := amqp.Dial(a.url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err == nil {
		err = declare(ch)
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	a.conn, a.ch = conn, ch
	return ch, nil
}

// Publish sends one persistent message.
func (a *AlertPublisher) Publish(ctx context.Context, m LowBalance) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	ch, err := a.channel()
	if err != nil {
		return fmt.Errorf("rabbitmq connect: %w", err)
	}
	err = ch.PublishWithContext(ctx, "", LowBalanceQueue, false, false,
		amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body})
	if err != nil {
		_ = a.conn.Close()
		a.conn, a.ch = nil, nil
		return fmt.Errorf("rabbitmq publish: %w", err)
	}
	return nil
}

// Close closes the connection.
func (a *AlertPublisher) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn != nil {
		_ = a.conn.Close()
	}
}

// ConsumeLowBalance calls handle for every alert until ctx is cancelled,
// reconnecting when the broker goes away.
func ConsumeLowBalance(ctx context.Context, url string, handle func(LowBalance)) {
	for ctx.Err() == nil {
		if err := consumeOnce(ctx, url, handle); err != nil {
			log.Printf("alert consumer: %v, retrying in 3s", err)
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func consumeOnce(ctx context.Context, url string, handle func(LowBalance)) error {
	conn, err := amqp.Dial(url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := declare(ch); err != nil {
		return err
	}
	msgs, err := ch.Consume(LowBalanceQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			var m LowBalance
			if err := json.Unmarshal(d.Body, &m); err != nil {
				log.Printf("dropping undecodable alert: %v", err)
				_ = d.Reject(false)
				continue
			}
			handle(m)
			_ = d.Ack(false)
		}
	}
}
