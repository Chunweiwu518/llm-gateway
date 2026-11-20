package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// UsageTopicPartitions is the partition count EnsureTopic gives a new usage topic.
const UsageTopicPartitions = 3

// EnsureTopic creates the topic when it does not exist yet and retries while the broker is starting.
// The consumer calls it before joining its group: if the group joined first and the broker auto-created
// the topic a moment later, the group would be assigned no partitions and never see a message.
// The replication factor is 1 (single broker); change it together with the broker count.
func EnsureTopic(ctx context.Context, brokers []string, topic string, partitions int) error {
	var err error
	for attempt := 0; attempt < 30; attempt++ {
		if err = createTopic(ctx, brokers[0], topic, partitions); err == nil || errors.Is(err, kafka.TopicAlreadyExists) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("ensure topic: %w", err)
}

func createTopic(ctx context.Context, broker, topic string, partitions int) error {
	d := &kafka.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	ctrl, err := conn.Controller()
	if err != nil {
		return err
	}
	cc, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ctrl.Host, strconv.Itoa(ctrl.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = cc.Close() }()
	return cc.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: partitions, ReplicationFactor: 1})
}
