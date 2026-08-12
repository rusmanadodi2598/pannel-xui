// Package telegramhandler test covers routing: /start and callback navigation.
//
// @file      internal/handler/telegram/dispatcher_test.go
// @for       Fakes/helpers shared with gate tests + start/menu-callback/noop routing.
// @uses      testing, context, log/slog, io, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Split from dispatcher_gate_test.go to respect the 250-line limit (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

const groupLink = "https://t.me/kentangtech"

var errBoom = errors.New("boom")

type fakeAPI struct {
	sent     []sendCall
	edited   []editCall
	answered []string
	docs     []sendDocCall
}

type sendCall struct {
	chatID int64
	text   string
	markup models.ReplyMarkup
}

type editCall struct {
	chatID    int64
	messageID int
	text      string
	markup    models.ReplyMarkup
}

type sendDocCall struct {
	chatID   int64
	filename string
	content  []byte
	caption  string
}

func (f *fakeAPI) SendMessage(_ context.Context, chatID int64, text string, _ models.ParseMode, markup models.ReplyMarkup) error {
	f.sent = append(f.sent, sendCall{chatID: chatID, text: text, markup: markup})
	return nil
}

func (f *fakeAPI) EditMessageText(_ context.Context, chatID int64, messageID int, text string, _ models.ParseMode, markup models.ReplyMarkup) error {
	f.edited = append(f.edited, editCall{chatID: chatID, messageID: messageID, text: text, markup: markup})
	return nil
}

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, callbackID, text string) error {
	f.answered = append(f.answered, callbackID+":"+text)
	return nil
}

func (f *fakeAPI) SendDocument(_ context.Context, chatID int64, filename string, content []byte, caption string) error {
	f.docs = append(f.docs, sendDocCall{chatID: chatID, filename: filename, content: content, caption: caption})
	return nil
}

type fakeGate struct {
	enabled bool
	result  telegramservice.GateResult
	calls   int
}

func (f *fakeGate) Enabled() bool { return f.enabled }
func (f *fakeGate) Check(context.Context, int64) telegramservice.GateResult {
	f.calls++
	return f.result
}
func (f *fakeGate) CheckFresh(context.Context, int64) telegramservice.GateResult {
	f.calls++
	return f.result
}

type fakeBan struct {
	banned bool
	err    error
}

func (f *fakeBan) IsBanned(context.Context, int64) (bool, error) { return f.banned, f.err }

type fakeLimiter struct {
	allow bool
	err   error
}

func (f *fakeLimiter) Allow(context.Context, int64) (bool, error) { return f.allow, f.err }

func newDispatcher(api API, gate GateChecker, ban BanChecker, lim RateLimiter) *Dispatcher {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewDispatcher(api, gate, ban, lim, logger, groupLink, nil, nil, nil, nil)
}

func newDispatcherWithAdmins(api API, gate GateChecker, ban BanChecker, lim RateLimiter, admins []int64) *Dispatcher {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewDispatcher(api, gate, ban, lim, logger, groupLink, admins, nil, nil, nil)
}

func msgUpdate(uid int64, text string) *models.Update {
	return &models.Update{ID: 1, Message: &models.Message{
		ID:   10,
		Text: text,
		From: &models.User{ID: uid, FirstName: "Dodi"},
		Chat: models.Chat{ID: uid, Type: "private"},
	}}
}

func cbUpdate(uid int64, data string) *models.Update {
	return &models.Update{ID: 2, CallbackQuery: &models.CallbackQuery{
		ID:   "cb-1",
		From: models.User{ID: uid, FirstName: "Dodi"},
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: 10, Chat: models.Chat{ID: uid, Type: "private"}},
		},
		Data: data,
	}}
}

func TestHandle_GivenStart_ThenHomeMenu(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), msgUpdate(7, "/start"))
	if len(api.sent) != 1 || api.sent[0].chatID != 7 {
		t.Fatalf("sent = %+v", api.sent)
	}
	if api.sent[0].text != telegramservice.HomeText("Dodi") {
		t.Errorf("home text mismatch")
	}
}

func TestHandle_GivenMenuCallback_ThenEditsInPlace(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, telegramservice.CallbackHome))

	if len(api.edited) != 1 || api.edited[0].messageID != 10 || api.edited[0].chatID != 7 {
		t.Fatalf("edited = %+v", api.edited)
	}
	if len(api.answered) != 1 {
		t.Fatalf("expected callback answered, got %+v", api.answered)
	}
	if len(api.sent) != 0 {
		t.Fatalf("menu callback must edit, not send: %+v", api.sent)
	}
}

func TestHandle_GivenUnknownCallback_ThenNoopAnswer(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), cbUpdate(7, "buy:menu"))
	if len(api.answered) != 1 {
		t.Fatalf("answered = %+v", api.answered)
	}
	if len(api.edited) != 0 {
		t.Fatalf("unknown callback must not edit: %+v", api.edited)
	}
}

func TestHandle_GivenUpdateWithoutUser_ThenIgnored(t *testing.T) {
	api := &fakeAPI{}
	d := newDispatcher(api, &fakeGate{}, &fakeBan{}, &fakeLimiter{allow: true})
	d.Handle(context.Background(), &models.Update{ID: 3, Message: &models.Message{ID: 1, Chat: models.Chat{ID: 1, Type: "private"}}})
	if len(api.sent) != 0 && len(api.answered) != 0 {
		t.Fatalf("update without user must be ignored: %+v %+v", api.sent, api.answered)
	}
}
