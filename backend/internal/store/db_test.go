package store

import "testing"

func TestSeedAdminIdempotent(t *testing.T) {
	db := OpenTest(t)
	for i := 0; i < 2; i++ {
		if err := SeedAdmin(db, "admin@example.com", "admin-pass-123"); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	var users []User
	db.Find(&users)
	if len(users) != 1 || users[0].Role != RoleAdmin {
		t.Fatalf("want 1 admin, got %+v", users)
	}
}
