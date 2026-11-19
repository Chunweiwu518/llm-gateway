// Package billing computes call cost and moves prepaid balance.
// All amounts are integer micro-USD (store.MicroPerUSD per dollar).
package billing

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

// ErrInsufficient means the balance cannot cover the amount.
var ErrInsufficient = errors.New("insufficient balance")

// Cost returns ceil((prompt*in + completion*out) / 1M) in micro-USD.
func Cost(p store.ModelPrice, prompt, completion int) int64 {
	total := int64(prompt)*p.InputPerM + int64(completion)*p.OutputPerM
	return (total + 999_999) / 1_000_000
}

// Reserve atomically deducts amount before an upstream call. It fails with
// ErrInsufficient when the balance is below amount, or is zero or less.
// The conditional UPDATE makes concurrent reservations unable to overdraw.
func Reserve(db *gorm.DB, userID uint, amount int64) error {
	if amount < 0 {
		return errors.New("reserve amount must not be negative")
	}
	floor := max(amount, 1)
	res := db.Model(&store.User{}).Where("id = ? AND balance >= ?", userID, floor).
		UpdateColumn("balance", gorm.Expr("balance - ?", amount))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInsufficient
	}
	return nil
}

// Settle returns reserved-actual to the user. When the call cost more than the
// reservation, it charges the extra only up to the remaining balance, so the
// balance never goes below zero. It returns the amount actually charged.
func Settle(db *gorm.DB, userID uint, reserved, actual int64) (int64, error) {
	if actual <= reserved {
		if actual == reserved {
			return actual, nil
		}
		err := db.Model(&store.User{}).Where("id = ?", userID).
			UpdateColumn("balance", gorm.Expr("balance + ?", reserved-actual)).Error
		return actual, err
	}
	charged := reserved
	err := db.Transaction(func(tx *gorm.DB) error {
		var u store.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID).Error; err != nil {
			return err
		}
		extra := min(actual-reserved, max(u.Balance, 0))
		charged += extra
		return tx.Model(&u).UpdateColumn("balance", gorm.Expr("balance - ?", extra)).Error
	})
	return charged, err
}

// Topup adds amount (negative for an admin deduction) and records it.
// A deduction that would make the balance negative fails with ErrInsufficient.
func Topup(db *gorm.DB, userID uint, amount int64, source string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&store.User{}).Where("id = ? AND balance + ? >= 0", userID, amount).
			UpdateColumn("balance", gorm.Expr("balance + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInsufficient
		}
		return tx.Create(&store.Topup{UserID: userID, Amount: amount, Source: source}).Error
	})
}
