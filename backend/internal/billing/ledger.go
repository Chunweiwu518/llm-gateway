// Package billing computes call cost and moves prepaid balance.
// All amounts are integer micro-USD (store.MicroPerUSD per dollar).
package billing

import (
	"errors"

	"gorm.io/gorm"

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

// Settle returns reserved-actual to the user (or charges the extra when the
// call cost more than the estimate).
func Settle(db *gorm.DB, userID uint, reserved, actual int64) error {
	diff := reserved - actual
	if diff == 0 {
		return nil
	}
	return db.Model(&store.User{}).Where("id = ?", userID).
		UpdateColumn("balance", gorm.Expr("balance + ?", diff)).Error
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
