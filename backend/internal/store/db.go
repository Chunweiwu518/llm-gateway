package store

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to MySQL and migrates the schema.
func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.AutoMigrate(&User{}, &APIKey{}, &Channel{}, &ModelPrice{}, &UsageLog{}, &Topup{}); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// SeedAdmin creates the admin account if no user with that email exists.
func SeedAdmin(db *gorm.DB, email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	var u User
	err := db.Where("email = ?", email).First(&u).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return db.Create(&User{Email: email, PasswordHash: hash, Role: RoleAdmin}).Error
}
