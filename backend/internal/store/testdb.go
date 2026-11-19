package store

import (
	"os"
	"testing"

	"gorm.io/gorm"
)

// OpenTest opens the test database from TEST_MYSQL_DSN and empties every table.
// The test is skipped when the variable is not set.
func OpenTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	for _, m := range []any{&UsageLog{}, &Topup{}, &APIKey{}, &Channel{}, &ModelPrice{}, &User{}} {
		if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	return db
}
