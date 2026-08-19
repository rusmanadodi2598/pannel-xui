// Package httphandler tests cover the admin REST API (PRD §26.5).
//
// @file      internal/handler/http/api_test.go
// @for       X-API-Key auth + servers CRUD, orders/users read, topup trigger.
// @uses      testing, net/http/httptest, context, io, log/slog, time, errors,
// internal/domain, internal/repository/postgres, internal/service/topup, gorm.io/gorm
// @reason    Guards the deferred admin surface contract (auth, envelope, status
// codes, no-credential reads) without a live DB.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
	"gorm.io/gorm"
)

const testAPIKey = "test-api-key"

// fakeServerAdmin implements ServerAdmin.
type fakeServerAdmin struct {
	all       []postgres.ServerAdminView
	view      postgres.ServerAdminView
	viewErr   error
	addID     int64
	addErr    error
	updateErr error
	deleteErr error
	health    string
}

func (f *fakeServerAdmin) ListAll(context.Context) ([]postgres.ServerAdminView, error) {
	return f.all, nil
}
func (f *fakeServerAdmin) GetAdminByID(context.Context, int64) (postgres.ServerAdminView, error) {
	return f.view, f.viewErr
}
func (f *fakeServerAdmin) AddServer(context.Context, serversvc.NewServerInput) (int64, error) {
	return f.addID, f.addErr
}
func (f *fakeServerAdmin) UpdateServer(context.Context, serversvc.UpdateServerInput) error {
	return f.updateErr
}
func (f *fakeServerAdmin) DeleteServer(context.Context, int64) error { return f.deleteErr }
func (f *fakeServerAdmin) CheckHealth(context.Context, int64) (string, error) {
	return f.health, nil
}

// fakeOrderAdmin implements OrderAdmin.
type fakeOrderAdmin struct {
	stats    postgres.OrderStats
	recent   []postgres.Order
	order    *postgres.Order
	orderErr error
	list     []postgres.Order
	count    int64
}

func (f *fakeOrderAdmin) Stats(context.Context, *time.Location) (postgres.OrderStats, error) {
	return f.stats, nil
}
func (f *fakeOrderAdmin) RecentOrders(context.Context, int) ([]postgres.Order, error) {
	return f.recent, nil
}
func (f *fakeOrderAdmin) GetByOrderID(context.Context, string) (*postgres.Order, error) {
	return f.order, f.orderErr
}
func (f *fakeOrderAdmin) ListByUserPage(context.Context, int64, int, int) ([]postgres.Order, error) {
	return f.list, nil
}
func (f *fakeOrderAdmin) CountByUser(context.Context, int64) (int64, error) { return f.count, nil }

// fakeClientReader implements ClientReader.
type fakeClientReader struct {
	list  []postgres.ClientView
	count int64
}

func (f *fakeClientReader) ListByUserPage(context.Context, int64, int, int) ([]postgres.ClientView, error) {
	return f.list, nil
}
func (f *fakeClientReader) CountByUser(context.Context, int64) (int64, error) { return f.count, nil }

// fakeUserResolver implements UserResolver.
type fakeUserResolver struct {
	user *postgres.User
	err  error
}

func (f *fakeUserResolver) GetByTelegramID(context.Context, int64) (*postgres.User, error) {
	return f.user, f.err
}

// fakeTopupTrigger implements TopupTrigger.
type fakeTopupTrigger struct {
	quote     topupsvc.Quote
	quoteErr  error
	result    *topupsvc.PaymentResult
	createErr error
}

func (f *fakeTopupTrigger) Quote(domain.Money) (topupsvc.Quote, error) {
	return f.quote, f.quoteErr
}
func (f *fakeTopupTrigger) CreatePayment(context.Context, topupsvc.CreatePaymentRequest) (*topupsvc.PaymentResult, error) {
	return f.result, f.createErr
}

func newAPIHandler(s ServerAdmin, o OrderAdmin, c ClientReader, u UserResolver, tp TopupTrigger) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{
		Logger:        logger,
		Version:       "test",
		WebhookPath:   "/api/v1/webhooks/telegram",
		WebhookSecret: testSecret,
		DB:            fakePinger{},
		Redis:         fakePinger{},
		RESTAPIKey:    testAPIKey,
		Servers:       s,
		Orders:        o,
		Clients:       c,
		Users:         u,
		Topups:        tp,
	})
}

func authHeader() map[string]string { return map[string]string{"X-API-Key": testAPIKey} }

func TestAPI_GivenMissingKey_Then401(t *testing.T) {
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPI_GivenWrongKey_Then401(t *testing.T) {
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers", map[string]string{"X-API-Key": "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPI_GivenNoRESTKeyConfigured_Then404(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Options{Logger: logger, Version: "test", WebhookPath: "/api/v1/webhooks/telegram", WebhookSecret: testSecret, RESTAPIKey: ""})
	rec := do(t, h, http.MethodGet, "/api/v1/servers", authHeader())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (surface disabled)", rec.Code)
	}
}

func TestAPI_ListServers_Then200WithEnvelope(t *testing.T) {
	s := &fakeServerAdmin{all: []postgres.ServerAdminView{{ID: 7, Name: "sg", Protocols: `["vless"]`}}}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"protocols":["vless"]`) {
		t.Errorf("body = %s, want parsed protocols array", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"meta"`) {
		t.Errorf("body = %s, want list meta", rec.Body.String())
	}
}

func TestAPI_CreateServer_Then201(t *testing.T) {
	s := &fakeServerAdmin{addID: 99}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := doBody(t, h, http.MethodPost, "/api/v1/servers",
		`{"name":"jp","host":"h","port":2053,"username":"u","password":"p","countryCode":"JP"}`,
		authHeader())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestAPI_GetServer_Then200NoPassword(t *testing.T) {
	s := &fakeServerAdmin{view: postgres.ServerAdminView{ID: 7, Name: "sg", Host: "h", Port: 2053}}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers/7", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "username") {
		t.Errorf("body leaks credentials: %s", rec.Body.String())
	}
}

func TestAPI_GetServer_GivenMissing_Then404(t *testing.T) {
	s := &fakeServerAdmin{viewErr: postgres.ErrServerNotFound}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers/9", authHeader())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAPI_DeleteServer_GivenHasClients_Then409(t *testing.T) {
	s := &fakeServerAdmin{deleteErr: postgres.ErrServerHasClients}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodDelete, "/api/v1/servers/7", authHeader())
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAPI_DeleteServer_Then204(t *testing.T) {
	s := &fakeServerAdmin{}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodDelete, "/api/v1/servers/7", authHeader())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestAPI_ServerHealth_Then200(t *testing.T) {
	s := &fakeServerAdmin{health: "ok"}
	h := newAPIHandler(s, &fakeOrderAdmin{}, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/servers/7/health", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAPI_ListOrders_Then200(t *testing.T) {
	o := &fakeOrderAdmin{
		stats:  postgres.OrderStats{TotalOrders: 3, TotalRevenue: 12000},
		recent: []postgres.Order{{ID: 1, OrderID: "KTS-1-VPN", Status: "completed", Amount: 4000, FinalAmount: 4000}},
	}
	h := newAPIHandler(&fakeServerAdmin{}, o, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/orders", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"totalRevenue":12000`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAPI_GetOrder_Then200(t *testing.T) {
	o := &fakeOrderAdmin{order: &postgres.Order{ID: 1, OrderID: "KTS-1-VPN", Status: "completed", Amount: 4000, FinalAmount: 4000}}
	h := newAPIHandler(&fakeServerAdmin{}, o, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/orders/KTS-1-VPN", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAPI_GetOrder_GivenMissing_Then404(t *testing.T) {
	o := &fakeOrderAdmin{orderErr: gorm.ErrRecordNotFound}
	h := newAPIHandler(&fakeServerAdmin{}, o, &fakeClientReader{}, &fakeUserResolver{}, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/orders/KTS-x", authHeader())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAPI_UserOrders_Then200(t *testing.T) {
	u := &fakeUserResolver{user: &postgres.User{ID: 42, TelegramID: 555}}
	o := &fakeOrderAdmin{list: []postgres.Order{{ID: 1, OrderID: "KTS-1-VPN", Status: "completed"}}, count: 1}
	h := newAPIHandler(&fakeServerAdmin{}, o, &fakeClientReader{}, u, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/users/555/orders", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAPI_UserOrders_GivenUnknownUser_Then404(t *testing.T) {
	u := &fakeUserResolver{err: gorm.ErrRecordNotFound}
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, &fakeClientReader{}, u, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/users/555/orders", authHeader())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAPI_UserClients_Then200CredentialFree(t *testing.T) {
	u := &fakeUserResolver{user: &postgres.User{ID: 42, TelegramID: 555}}
	c := &fakeClientReader{list: []postgres.ClientView{{VPNClient: postgres.VPNClient{ID: 1, Email: "a@b", UUID: "secret-uuid", SubID: "sub-secret"}}}, count: 1}
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, c, u, &fakeTopupTrigger{})
	rec := do(t, h, http.MethodGet, "/api/v1/users/555/clients", authHeader())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret-uuid") || strings.Contains(rec.Body.String(), "sub-secret") {
		t.Errorf("client response leaks credentials: %s", rec.Body.String())
	}
}

func TestAPI_CreateTopup_Then201(t *testing.T) {
	u := &fakeUserResolver{user: &postgres.User{ID: 42, TelegramID: 555, FirstName: "A", Username: "b"}}
	tp := &fakeTopupTrigger{
		quote:  topupsvc.Quote{Net: 10000, Gross: 10300},
		result: &topupsvc.PaymentResult{OrderID: "tp_1", CheckoutURL: "https://pay", Amount: 10300},
	}
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, &fakeClientReader{}, u, tp)
	rec := doBody(t, h, http.MethodPost, "/api/v1/payments/topups",
		`{"telegramId":555,"amount":10000}`, authHeader())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"orderId":"tp_1"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestAPI_CreateTopup_GivenInvalidAmount_Then422(t *testing.T) {
	u := &fakeUserResolver{user: &postgres.User{ID: 42, TelegramID: 555}}
	tp := &fakeTopupTrigger{quoteErr: topupsvc.ErrInvalidNominal}
	h := newAPIHandler(&fakeServerAdmin{}, &fakeOrderAdmin{}, &fakeClientReader{}, u, tp)
	rec := doBody(t, h, http.MethodPost, "/api/v1/payments/topups",
		`{"telegramId":555,"amount":1}`, authHeader())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}
