// Package telegram wraps the go-telegram/bot library at the repository layer.
//
// @file      internal/repository/telegram/api.go
// @for       Typed Telegram API client: webhook registration, messaging, chat membership.
// @uses      github.com/go-telegram/bot, github.com/go-telegram/bot/models, net/http, context, time, fmt
// @reason    Keeps the third-party client behind one seam so services depend on interfaces, not the SDK (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// defaultTelegramAPI is the production Telegram Bot API base URL.
const defaultTelegramAPI = "https://api.telegram.org"

// Client is the typed seam over *bot.Bot. It is the only place the SDK is
// imported, mirroring the repository/xui pattern for the panel REST client.
type Client struct {
	b *bot.Bot
}

// NewClient creates the bot, validates the token via getMe (fail fast on boot)
// and configures an explicit HTTP timeout (AGENTS.md §1.6).
func NewClient(token string, timeout time.Duration) (*Client, error) {
	return newClient(token, defaultTelegramAPI, timeout)
}

// NewClientWithServerURL is the test seam: it points the SDK at a fake
// Telegram API server (httptest) instead of the production endpoint.
func NewClientWithServerURL(token, serverURL string, timeout time.Duration) (*Client, error) {
	return newClient(token, serverURL, timeout)
}

func newClient(token, serverURL string, timeout time.Duration) (*Client, error) {
	httpClient := &http.Client{Timeout: timeout}
	b, err := bot.New(token,
		bot.WithServerURL(serverURL),
		bot.WithHTTPClient(timeout, httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init (getMe): %w", err)
	}
	return &Client{b: b}, nil
}

// SetWebhook registers the webhook URL with Telegram (PRD §14.1).
func (c *Client) SetWebhook(ctx context.Context, url, secret string, allowedUpdates []string, dropPending bool, maxConnections int) error {
	ok, err := c.b.SetWebhook(ctx, &bot.SetWebhookParams{
		URL:                url,
		SecretToken:        secret,
		AllowedUpdates:     allowedUpdates,
		DropPendingUpdates: dropPending,
		MaxConnections:     maxConnections,
	})
	if err != nil {
		return fmt.Errorf("telegram setWebhook: %w", err)
	}
	if !ok {
		return fmt.Errorf("telegram setWebhook: api returned ok=false")
	}
	return nil
}

// WebhookInfo returns the current webhook configuration from Telegram.
func (c *Client) WebhookInfo(ctx context.Context) (*models.WebhookInfo, error) {
	info, err := c.b.GetWebhookInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("telegram getWebhookInfo: %w", err)
	}
	return info, nil
}

// SendMessage sends a text message (with optional inline keyboard) to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error {
	if _, err := c.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   parseMode,
		ReplyMarkup: markup,
	}); err != nil {
		return fmt.Errorf("telegram sendMessage: %w", err)
	}
	return nil
}

// EditMessageText replaces the text/keyboard of an existing message in place.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error {
	if _, err := c.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   parseMode,
		ReplyMarkup: markup,
	}); err != nil {
		return fmt.Errorf("telegram editMessageText: %w", err)
	}
	return nil
}

// AnswerCallbackQuery acknowledges an inline button tap (noop feedback).
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackID, text string) error {
	if _, err := c.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
	}); err != nil {
		return fmt.Errorf("telegram answerCallbackQuery: %w", err)
	}
	return nil
}

// GetChatMember returns the membership type of a user in a chat (group gate).
func (c *Client) GetChatMember(ctx context.Context, chatID, userID int64) (models.ChatMemberType, error) {
	member, err := c.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return "", fmt.Errorf("telegram getChatMember: %w", err)
	}
	return member.Type, nil
}
