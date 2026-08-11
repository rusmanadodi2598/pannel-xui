// Package telegram test covers webhook registration against a fake registrar.
//
// @file      internal/service/telegram/webhook_test.go
// @for       WebhookService: URL composition, setWebhook payload, getWebhookInfo verification.
// @uses      testing, context, github.com/go-telegram/bot/models
// @reason    Guards the PRD §14.1 webhook contract without network access.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
)

const testSecret = "0123456789abcdef0123456789abcdef"

type fakeRegistrar struct {
	url            string
	secret         string
	allowed        []string
	dropPending    bool
	maxConnections int
	setCalls       int
	infoCalls      int
	info           *models.WebhookInfo
	setErr         error
}

func (f *fakeRegistrar) SetWebhook(_ context.Context, url, secret string, allowed []string, dropPending bool, maxConnections int) error {
	f.setCalls++
	f.url, f.secret, f.allowed, f.dropPending, f.maxConnections = url, secret, allowed, dropPending, maxConnections
	return f.setErr
}

func (f *fakeRegistrar) WebhookInfo(context.Context) (*models.WebhookInfo, error) {
	f.infoCalls++
	return f.info, nil
}

func TestWebhookURL_GivenDomainAndPath_ThenComposesHTTPS(t *testing.T) {
	svc := NewWebhookService(&fakeRegistrar{}, "bot-xui.kentangtechstore.com", "/api/v1/webhooks/telegram", testSecret, 40, true)
	want := "https://bot-xui.kentangtechstore.com/api/v1/webhooks/telegram"
	if got := svc.WebhookURL(); got != want {
		t.Errorf("WebhookURL = %q, want %q", got, want)
	}
}

func TestWebhookURL_GivenTrailingSlashes_ThenNormalized(t *testing.T) {
	svc := NewWebhookService(&fakeRegistrar{}, "bot-xui.kentangtechstore.com/", "api/v1/webhooks/telegram/", testSecret, 40, true)
	if got := svc.WebhookURL(); got != "https://bot-xui.kentangtechstore.com/api/v1/webhooks/telegram/" {
		t.Errorf("WebhookURL = %q", got)
	}
}

func TestRegister_GivenHealthyAPI_ThenSetsAndVerifies(t *testing.T) {
	f := &fakeRegistrar{info: &models.WebhookInfo{URL: "https://x/api/v1/webhooks/telegram", PendingUpdateCount: 3}}
	svc := NewWebhookService(f, "bot-xui.kentangtechstore.com", "/api/v1/webhooks/telegram", testSecret, 40, true)

	info, err := svc.Register(context.Background())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if f.setCalls != 1 || f.infoCalls != 1 {
		t.Errorf("calls set=%d info=%d, want 1/1", f.setCalls, f.infoCalls)
	}
	if f.url != "https://bot-xui.kentangtechstore.com/api/v1/webhooks/telegram" {
		t.Errorf("url = %q", f.url)
	}
	if f.secret != testSecret || !f.dropPending || f.maxConnections != 40 {
		t.Errorf("setWebhook payload = secret:%q drop:%v max:%d", f.secret, f.dropPending, f.maxConnections)
	}
	if len(f.allowed) != 2 || f.allowed[0] != "message" || f.allowed[1] != "callback_query" {
		t.Errorf("allowed updates = %v", f.allowed)
	}
	if info.PendingUpdateCount != 3 {
		t.Errorf("info = %+v", info)
	}
}

func TestRegister_GivenSetWebhookError_ThenFailsFast(t *testing.T) {
	f := &fakeRegistrar{setErr: errBoom}
	svc := NewWebhookService(f, "d.example.com", "/api/v1/webhooks/telegram", testSecret, 40, true)
	if _, err := svc.Register(context.Background()); err == nil {
		t.Fatal("expected error when setWebhook fails")
	}
	if f.infoCalls != 0 {
		t.Error("getWebhookInfo must not be called after setWebhook failure")
	}
}
