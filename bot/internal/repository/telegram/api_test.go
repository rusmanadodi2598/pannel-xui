// Package telegram test verifies the client against a fake Bot API server.
//
// @file      internal/repository/telegram/api_test.go
// @for       httptest fake Bot API: getMe, setWebhook, getWebhookInfo, send/edit/answer, getChatMember.
// @uses      testing, net/http/httptest, encoding/json, context, time, github.com/go-telegram/bot/models
// @reason    Locks the exact wire contract (paths, JSON bodies) the bot depends on (PRD §14).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

const testToken = "123456:TESTTOKEN"

type fakeAPI struct {
	mux        *http.ServeMux
	formValues map[string]string
	lastMethod string
	lastPath   string
	lastFile   string
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	t.Helper()
	f := &fakeAPI{mux: http.NewServeMux(), formValues: map[string]string{}}
	f.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// go-telegram/bot sends form-encoded params (multipart when files exist).
		_ = r.ParseMultipartForm(1 << 20)
		for k, v := range r.Form {
			f.formValues[k] = v[0]
		}
		if r.MultipartForm != nil {
			for name, files := range r.MultipartForm.File {
				if len(files) > 0 {
					f.lastFile = name + "=" + files[0].Filename
				}
			}
		}
		f.lastMethod = r.Method
		f.lastPath = r.URL.Path
		writeResult(w, f.route(r.URL.Path))
	})
	srv := httptest.NewServer(f.mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func writeResult(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

// route answers each Bot API method with the minimal JSON the SDK parses.
func (f *fakeAPI) route(path string) any {
	switch {
	case strings.HasSuffix(path, "/getMe"):
		return map[string]any{"id": 123456, "is_bot": true, "first_name": "TestBot", "username": "testbot"}
	case strings.HasSuffix(path, "/setWebhook"):
		return true
	case strings.HasSuffix(path, "/getWebhookInfo"):
		return map[string]any{"url": "https://bot-xui.kentangtechstore.com/api/v1/webhooks/telegram", "pending_update_count": 3}
	case strings.HasSuffix(path, "/sendMessage"), strings.HasSuffix(path, "/editMessageText"),
		strings.HasSuffix(path, "/sendDocument"):
		return map[string]any{"message_id": 1, "chat": map[string]any{"id": 42, "type": "private"}}
	case strings.HasSuffix(path, "/answerCallbackQuery"):
		return true
	case strings.HasSuffix(path, "/getChatMember"):
		return map[string]any{"status": "member", "user": map[string]any{"id": 7, "is_bot": false, "first_name": "T"}}
	default:
		return map[string]any{}
	}
}

func newTestClient(t *testing.T) (*Client, *fakeAPI) {
	t.Helper()
	f, srv := newFakeAPI(t)
	c, err := NewClientWithServerURL(testToken, srv.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewClientWithServerURL: %v", err)
	}
	return c, f
}

func TestNewClient_GivenInvalidToken_ThenError(t *testing.T) {
	_, srv := newFakeAPI(t)
	if _, err := NewClientWithServerURL("", srv.URL, time.Second); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestSetWebhook_GivenParams_ThenSendsExactJSON(t *testing.T) {
	c, f := newTestClient(t)
	err := c.SetWebhook(context.Background(),
		"https://bot-xui.kentangtechstore.com/api/v1/webhooks/telegram",
		"super-secret-32-chars-minimum-xxxx",
		[]string{"message", "callback_query"},
		true, 40)
	if err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	if !strings.HasSuffix(f.lastPath, "/setWebhook") {
		t.Fatalf("path = %s", f.lastPath)
	}
	if f.formValues["url"] == "" || f.formValues["secret_token"] == "" ||
		f.formValues["drop_pending_updates"] != "true" || f.formValues["max_connections"] != "40" {
		t.Errorf("unexpected setWebhook form: %+v", f.formValues)
	}
	var allowed []string
	if err := json.Unmarshal([]byte(f.formValues["allowed_updates"]), &allowed); err != nil || len(allowed) != 2 {
		t.Errorf("allowed_updates = %q (err %v)", f.formValues["allowed_updates"], err)
	}
}

func TestWebhookInfo_GivenRegistered_ThenReturnsInfo(t *testing.T) {
	c, _ := newTestClient(t)
	info, err := c.WebhookInfo(context.Background())
	if err != nil {
		t.Fatalf("WebhookInfo: %v", err)
	}
	if info.URL == "" || info.PendingUpdateCount != 3 {
		t.Errorf("info = %+v", info)
	}
}

func TestSendMessage_GivenTextAndMarkup_ThenRoundTrips(t *testing.T) {
	c, f := newTestClient(t)
	keyboard := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Beli VPN", CallbackData: "buy:menu"}},
		},
	}
	if err := c.SendMessage(context.Background(), 42, "Halo", models.ParseModeHTML, keyboard); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if f.formValues["chat_id"] != "42" || f.formValues["text"] != "Halo" || f.formValues["parse_mode"] != "HTML" {
		t.Errorf("unexpected sendMessage form: %+v", f.formValues)
	}
	if !strings.Contains(f.formValues["reply_markup"], "buy:menu") {
		t.Errorf("reply_markup = %q, want inline keyboard with buy:menu", f.formValues["reply_markup"])
	}
}

func TestGetChatMember_GivenMember_ThenReturnsType(t *testing.T) {
	c, _ := newTestClient(t)
	mtype, err := c.GetChatMember(context.Background(), -100123456789, 7)
	if err != nil {
		t.Fatalf("GetChatMember: %v", err)
	}
	if mtype != models.ChatMemberTypeMember {
		t.Errorf("type = %q, want member", mtype)
	}
}

func TestAnswerCallbackQuery_GivenNoop_ThenSucceeds(t *testing.T) {
	c, f := newTestClient(t)
	if err := c.AnswerCallbackQuery(context.Background(), "cb-1", "ok"); err != nil {
		t.Fatalf("AnswerCallbackQuery: %v", err)
	}
	if f.formValues["callback_query_id"] != "cb-1" || f.formValues["text"] != "ok" {
		t.Errorf("payload = %+v", f.formValues)
	}
}

func TestSendDocument_GivenTXTBytes_ThenMultipartUpload(t *testing.T) {
	c, f := newTestClient(t)
	content := []byte("=== AKUN VPN ===\nvless://u@h:443")
	if err := c.SendDocument(context.Background(), 42, "akun-a-at-vpn-kt.txt", content, "Akun VPN kamu"); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	if !strings.HasSuffix(f.lastPath, "/sendDocument") {
		t.Fatalf("path = %s, want sendDocument", f.lastPath)
	}
	if f.formValues["chat_id"] != "42" || f.formValues["caption"] != "Akun VPN kamu" {
		t.Errorf("form = %+v", f.formValues)
	}
	if f.lastFile != "document=akun-a-at-vpn-kt.txt" {
		t.Errorf("uploaded file = %q, want document=akun-a-at-vpn-kt.txt", f.lastFile)
	}
}
