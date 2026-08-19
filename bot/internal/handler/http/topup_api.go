// Package httphandler also hosts the POST /api/v1/payments/topups trigger (PRD §26.5).
//
// @file      internal/handler/http/topup_api.go
// @for       Admin trigger: create a topup charge for a user's Telegram id.
// @uses      errors, net/http, gorm.io/gorm, internal/domain, internal/service/topup
// @reason    Programmatic topup initiation reuses the exact FR-06 charge flow
// (Quote + CreatePayment), so gross/fee and persistence stay in one place.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"errors"
	"net/http"

	"github.com/kentangtech/bot-order/internal/domain"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
	"gorm.io/gorm"
)

// createTopupRequest is the POST /payments/topups body (amount = NET rupiah).
type createTopupRequest struct {
	TelegramID int64 `json:"telegramId"`
	Amount     int64 `json:"amount"`
}

func (o Options) createTopup(w http.ResponseWriter, r *http.Request) {
	var req createTopupRequest
	if err := decodeBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Malformed request body")
		return
	}
	u, err := o.Users.GetByTelegramID(r.Context(), req.TelegramID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeAPIError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}
	if err != nil {
		o.Logger.Error("api: resolving topup user", "telegramId", req.TelegramID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to resolve user")
		return
	}

	net := domain.Money(req.Amount)
	quote, err := o.Topups.Quote(net)
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "INVALID_AMOUNT", "Amount is outside the allowed range")
		return
	}
	result, err := o.Topups.CreatePayment(r.Context(), topupsvc.CreatePaymentRequest{
		TelegramUserID: req.TelegramID,
		FirstName:      u.FirstName,
		Username:       u.Username,
		NetAmount:      quote.Net,
		GrossAmount:    quote.Gross,
	})
	if err != nil {
		o.Logger.Error("api: creating topup", "telegramId", req.TelegramID, "error", err)
		writeAPIError(w, http.StatusBadGateway, "GATEWAY_ERROR", "Failed to create payment")
		return
	}
	writeData(w, http.StatusCreated, map[string]any{
		"orderId":     result.OrderID,
		"checkoutUrl": result.CheckoutURL,
		"amount":      result.Amount.Rupiah(),
		"expiresAt":   result.ExpiresAt,
	})
}
