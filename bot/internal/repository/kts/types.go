// Package kts implements the KentangTech payment gateway client (PG Aggregate).
//
// @file      internal/repository/kts/types.go
// @for       DTOs for PG charge create/confirm/verify + the pg.charge webhook.
// @uses      encoding/json, time
// @reason    Schema-first DTOs (CDD §2.4) mirroring gateway contracts 015/013:
// the bot's payment surface is one typed mapping away from the wire.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package kts

import (
	"encoding/json"
	"time"
)

// EventPGCharge is the merchant webhook event name (013 §1).
const EventPGCharge = "pg.charge"

// Money mirrors schema.Money: integer minor units + explicit currency.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// CreateChargeRequest creates a PG charge for the user's topup (015 §3.2).
// Amount is the NET balance the user receives; the gateway derives the gross
// charged via Midtrans (2.5% MDR + 11% PPN gross-up) itself.
type CreateChargeRequest struct {
	OrderID     string `json:"orderId"` // single E2E id: [A-Za-z0-9._-], 4..50
	Amount      Money  `json:"amount"`
	Description string `json:"description,omitempty"`
}

// Charge is the PG charge create/confirm/verify response shape (015 §3.2).
type Charge struct {
	OrderID           string    `json:"orderId"`
	Amount            Money     `json:"amount"` // GROSS charged via Midtrans
	Status            string    `json:"status"` // created|pending|paid|expired|failed|refunded
	PaymentMethod     string    `json:"paymentMethod"`
	ProviderReference string    `json:"providerReference,omitempty"`
	CheckoutURL       string    `json:"checkoutUrl,omitempty"`
	QRString          string    `json:"qrString,omitempty"`
	NetAmount         *Money    `json:"netAmount,omitempty"`
	GrossAmount       *Money    `json:"grossAmount,omitempty"`
	FeeAmount         *Money    `json:"feeAmount,omitempty"`
	PaidAt            time.Time `json:"paidAt,omitempty"`
	ExpiresAt         time.Time `json:"expiresAt"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
}

// Notification is the pg.charge webhook body (013 §3.4 / 015 §5.3): the
// gateway POSTs it to the bot's registered webhook_url when a charge reaches
// a terminal state (succeeded|failed|expired).
type Notification struct {
	EventType     string    `json:"eventType"` // always "pg.charge"
	OrderID       string    `json:"orderId"`
	RefID         string    `json:"refId"` // = OrderID (single E2E id)
	Status        string    `json:"status"`
	Amount        Money     `json:"amount"` // GROSS — credit uses the local NET
	ProviderTrxID string    `json:"providerTrxId,omitempty"`
	PaidAt        time.Time `json:"paidAt,omitempty"`
	ErrorCode     string    `json:"errorCode,omitempty"`
	ErrorMsg      string    `json:"errorMsg,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
}

// apiEnvelope is the gateway's success envelope (001 §3): data + meta.
type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

// apiErrorEnvelope is the gateway's failure envelope: error + meta.
type apiErrorEnvelope struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
