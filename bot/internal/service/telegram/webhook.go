// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/webhook.go
// @for       setWebhook registration at boot + getWebhookInfo verification (PRD §14.1).
// @uses      context, strings, fmt, github.com/go-telegram/bot/models
// @reason    Owns the webhook contract so main.go stays a pure composition root.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

// AllowedUpdates is the update types registered with Telegram (PRD §14.1).
// Extended as new handler families land (chosen_inline_result, etc.).
var AllowedUpdates = []string{"message", "callback_query"}

// WebhookRegistrar is the Telegram seam consumed by WebhookService.
type WebhookRegistrar interface {
	SetWebhook(ctx context.Context, url, secret string, allowedUpdates []string, dropPending bool, maxConnections int) error
	WebhookInfo(ctx context.Context) (*models.WebhookInfo, error)
}

// WebhookService registers and verifies the bot webhook at startup.
type WebhookService struct {
	api            WebhookRegistrar
	domain         string
	path           string
	secret         string
	maxConnections int
	dropPending    bool
}

// NewWebhookService wires the service with the configured webhook contract.
func NewWebhookService(api WebhookRegistrar, domain, path, secret string, maxConnections int, dropPending bool) *WebhookService {
	return &WebhookService{
		api:            api,
		domain:         strings.TrimRight(domain, "/"),
		path:           "/" + strings.TrimLeft(path, "/"),
		secret:         secret,
		maxConnections: maxConnections,
		dropPending:    dropPending,
	}
}

// WebhookURL returns the public HTTPS URL registered with Telegram.
func (s *WebhookService) WebhookURL() string {
	return "https://" + s.domain + s.path
}

// Register calls setWebhook and verifies the result via getWebhookInfo.
// It fails fast so the process exits when Telegram cannot reach the bot.
func (s *WebhookService) Register(ctx context.Context) (*models.WebhookInfo, error) {
	if err := s.api.SetWebhook(ctx, s.WebhookURL(), s.secret, AllowedUpdates, s.dropPending, s.maxConnections); err != nil {
		return nil, fmt.Errorf("registering telegram webhook: %w", err)
	}
	info, err := s.api.WebhookInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("verifying telegram webhook: %w", err)
	}
	return info, nil
}
