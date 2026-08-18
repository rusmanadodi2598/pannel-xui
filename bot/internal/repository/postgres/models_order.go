// Package postgres also hosts the order-ledger models.
//
// @file      internal/repository/postgres/models_order.go
// @for       GORM models: `orders`, `balance_transactions`, `payments`, `pricing` (PRD §13.4-13.7).
// @uses      time, internal/domain
// @reason    Typed persistence of order state machine + immutable ledger (FR-04/FR-06, M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

// Order mirrors the `orders` table (PRD §13.4).
type Order struct {
	ID            int64        `gorm:"primaryKey"`
	OrderID       string       `gorm:"uniqueIndex;not null"` // KTS-XXXXXXXX-VPN
	OrderType     string       `gorm:"type:text;not null"`   // purchase|renewal|topup|trial|deletion
	UserID        int64        `gorm:"index:idx_orders_user_id;not null"`
	ServerID      *int64       `gorm:"index"`
	ClientID      *int64       `gorm:"index"`
	Protocol      string       `gorm:"type:text"`
	DurationDays  int          `gorm:"type:int"`
	TrafficGB     int          `gorm:"type:int"`
	IPLimit       int          `gorm:"type:int"`
	Amount        domain.Money `gorm:"not null"`
	Discount      domain.Money `gorm:"not null"`
	FinalAmount   domain.Money `gorm:"not null"`
	Currency      string       `gorm:"type:text;not null;default:IDR"`
	Status        string       `gorm:"index:idx_orders_status;type:text;not null;default:pending"`
	Notes         string       `gorm:"type:text"`
	ErrorMessage  string       `gorm:"type:text"`
	AccountEmail  string       `gorm:"type:text"`
	AccountRemark string       `gorm:"type:text"`
	BalanceBefore *domain.Money
	BalanceAfter  *domain.Money
	CompletedAt   *time.Time `gorm:"type:timestamptz"`
	CreatedAt     time.Time  `gorm:"index:idx_orders_created;type:timestamptz;not null;default:now()"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (Order) TableName() string { return "orders" }

// BalanceTransaction is one immutable ledger row (PRD §13.5).
type BalanceTransaction struct {
	ID           int64        `gorm:"primaryKey"`
	UserID       int64        `gorm:"index:idx_balance_tx_user;not null"`
	OrderID      string       `gorm:"type:text"`
	Type         string       `gorm:"type:text;not null"` // credit | debit
	Amount       domain.Money `gorm:"not null"`
	BalanceAfter domain.Money `gorm:"not null"`
	CreatedAt    time.Time    `gorm:"index:idx_balance_tx_created;type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (BalanceTransaction) TableName() string { return "balance_transactions" }

// Payment mirrors the `payments` table (PRD §13.6) — used by FR-06 (M5).
type Payment struct {
	ID             int64        `gorm:"primaryKey"`
	OrderID        string       `gorm:"uniqueIndex;not null"`
	UserID         int64        `gorm:"index:idx_payments_user;not null"`
	TelegramID     int64        `gorm:"index:idx_payments_telegram;not null;default:0"` // notif topup sukses (Phase 4)
	AmountGross    domain.Money `gorm:"not null"`
	AmountNet      domain.Money `gorm:"not null"`
	FeeAmount      domain.Money `gorm:"not null"`
	FeePct         float64      `gorm:"type:numeric(5,4);not null"`
	ProviderRef    string       `gorm:"type:text"`
	ProviderStatus string       `gorm:"type:text"`
	Status         string       `gorm:"index:idx_payments_status;type:text;not null;default:pending"`
	PaidAt         *time.Time   `gorm:"type:timestamptz"`
	CreatedAt      time.Time    `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time    `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (Payment) TableName() string { return "payments" }

// Pricing mirrors the `pricing` table (PRD §13.7) — seeded at boot (M4).
type Pricing struct {
	ID          int64        `gorm:"primaryKey"`
	CountryCode string       `gorm:"type:text;not null"`
	PlanDays    int          `gorm:"type:int;not null"`
	Price       domain.Money `gorm:"not null"`
	Enabled     bool         `gorm:"not null;default:true"`
	UpdatedAt   time.Time    `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (Pricing) TableName() string { return "pricing" }
