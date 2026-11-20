package events

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

func msg(t *testing.T, e store.UsageLog) kafka.Message {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return kafka.Message{Value: b}
}

func TestDecodeSkipsGarbageAndClearsID(t *testing.T) {
	rows := decode([]kafka.Message{
		msg(t, store.UsageLog{ID: 7, UserID: 1, Model: "m", Cost: 5}),
		{Value: []byte("not json")},
	})
	if len(rows) != 1 || rows[0].ID != 0 || rows[0].UserID != 1 || rows[0].Cost != 5 {
		t.Fatalf("rows %+v", rows)
	}
}

// When Kafka reports a failed delivery the publisher writes the rows to MySQL itself.
func TestFailedDeliveryFallsBackToMySQL(t *testing.T) {
	db := store.OpenTest(t)
	u := store.User{Email: "e@example.com", PasswordHash: "x"}
	db.Create(&u)
	p := &Publisher{db: db}
	at := time.Now().UTC().Truncate(time.Second)
	p.completed([]kafka.Message{msg(t, store.UsageLog{UserID: u.ID, APIKeyID: 1, Model: "m", Cost: 9, CreatedAt: at})}, errors.New("broker down"))
	var got store.UsageLog
	if err := db.Where("user_id = ?", u.ID).First(&got).Error; err != nil || got.Cost != 9 {
		t.Fatalf("row %+v err %v", got, err)
	}
	p.completed(nil, nil) // success path writes nothing
}
