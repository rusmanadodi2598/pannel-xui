// Package kts also hosts the PG charge HTTP client (spec 015).
//
// @file      internal/repository/kts/gateway.go
// @for       Create/confirm/verify PG charges over the merchant S2S chain.
// @uses      bytes, context, encoding/json, errors, fmt, io, net/http, net/url,
// strings, time, internal/domain
// @reason    The bot is a merchant on the PG Aggregate: every call carries
// X-API-Key/X-Timestamp/X-Nonce/X-Signature + Idempotency-Key (001 §2.3) and
// runs under an explicit timeout (AGENTS.md §1.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package kts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

// Typed gateway errors the topup service maps to user-facing messages.
var (
	ErrInvalidResponse = errors.New("invalid gateway response")
	ErrChargeNotFound  = errors.New("charge not found")
	ErrDuplicateOrder  = errors.New("order id already exists")
	ErrUnauthorized    = errors.New("gateway authentication failed")
)

// Client is the PG charge client.
type Client struct {
	baseURL string
	apiKey  string
	secret  string
	http    *http.Client
	now     func() time.Time
	nonce   func() string
}

// New builds the client; timeout bounds every outbound call (AGENTS.md §1.6).
func New(baseURL, apiKey, secret string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid gateway base url %q", baseURL)
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		secret:  secret,
		http:    &http.Client{Timeout: timeout},
		now:     time.Now,
		nonce:   func() string { return domain.NewUUID() },
	}, nil
}

// CreateCharge POST /api/v1/pg/charges — 201 on new charge, 200 on idempotent
// replay of the same orderId (015 §4.3).
func (c *Client) CreateCharge(ctx context.Context, req CreateChargeRequest) (*Charge, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding create charge: %w", err)
	}
	var out Charge
	if err := c.do(ctx, http.MethodPost, "/api/v1/pg/charges", req.OrderID, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConfirmCharge POST /api/v1/pg/charges/{orderId}/confirm — 202 pending plus
// the checkout URL (Midtrans QRIS action URL).
func (c *Client) ConfirmCharge(ctx context.Context, orderID string) (*Charge, error) {
	path := "/api/v1/pg/charges/" + url.PathEscape(orderID) + "/confirm"
	var out Charge
	if err := c.do(ctx, http.MethodPost, path, orderID+".confirm", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCharge GET /api/v1/pg/charges/{orderId} — verify/poll settlement.
func (c *Client) GetCharge(ctx context.Context, orderID string) (*Charge, error) {
	path := "/api/v1/pg/charges/" + url.PathEscape(orderID)
	var out Charge
	if err := c.do(ctx, http.MethodGet, path, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do signs and performs one S2S request, unwrapping the data envelope.
func (c *Client) do(ctx context.Context, method, path, idemKey string, body []byte, out *Charge) error {
	ts := c.now().Unix()
	nonce := c.nonce()
	canon := canonical(c.apiKey, fmt.Sprintf("%d", ts), nonce, method, path, body)

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building %s %s: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", sign(c.secret, canon))
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gateway %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading gateway response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.mapError(resp.StatusCode, raw)
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || len(env.Data) == 0 {
		return fmt.Errorf("%w: status %d", ErrInvalidResponse, resp.StatusCode)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("%w: decoding data", ErrInvalidResponse)
	}
	return nil
}

// mapError converts non-2xx into typed errors from the 001/015 error envelope.
func (c *Client) mapError(status int, raw []byte) error {
	var env apiErrorEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Error.Code != "" {
		switch env.Error.Code {
		case "DUPLICATE_ORDER":
			return fmt.Errorf("%w: %s", ErrDuplicateOrder, env.Error.Message)
		case "PAYMENT_NOT_FOUND":
			return fmt.Errorf("%w: %s", ErrChargeNotFound, env.Error.Message)
		case "UNAUTHORIZED", "INVALID_SIGNATURE", "REQUEST_EXPIRED", "REPLAY_DETECTED":
			return fmt.Errorf("%w: %s", ErrUnauthorized, env.Error.Code)
		default:
			return fmt.Errorf("gateway %s: %s", env.Error.Code, env.Error.Message)
		}
	}
	return fmt.Errorf("gateway responded %d", status)
}
