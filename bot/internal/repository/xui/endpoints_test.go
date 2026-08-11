// Package xui_test also covers the endpoint methods.
//
// @file      internal/repository/xui/endpoints_test.go
// @for       DeleteClient, traffic (array/single), error mapping, server status.
// @uses      testing, context, net/http, time
// @reason    Keeps client_test.go under 250 lines (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestDeleteClient_ThenOK(t *testing.T) {
	m := newMockPanel("admin", "secret")
	called := false
	m.register("/xui/API/inbounds/9/delClient/abc-123", func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeEnvelope(w, http.StatusOK, true, "Client deleted", nil)
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.DeleteClient(ctx, 9, "abc-123"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if !called {
		t.Error("delete route not hit")
	}
}

func TestGetClientTrafficByID_GivenArrayResponse_ThenParsed(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/inbounds/getClientTrafficsById/uuid-1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, true, "", []xui.ClientTraffic{
			{ID: 1, InboundID: 2, Email: "a@vpn.id", Up: 100, Down: 200, Total: 1024},
		})
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	list, err := c.GetClientTrafficByID(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("GetClientTrafficByID: %v", err)
	}
	if len(list) != 1 || list[0].Up != 100 || list[0].Down != 200 {
		t.Errorf("traffic = %+v", list)
	}
}

func TestGetClientTrafficByEmail_GivenSingleObject_ThenParsed(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/inbounds/getClientTraffics/a@vpn.id", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, true, "", xui.ClientTraffic{ID: 3, Email: "a@vpn.id", Total: 500})
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, err := c.GetClientTrafficByEmail(ctx, "a@vpn.id")
	if err != nil {
		t.Fatalf("GetClientTrafficByEmail: %v", err)
	}
	if tr.ID != 3 || tr.Total != 500 {
		t.Errorf("traffic = %+v", tr)
	}
}

func TestRequest_GivenPanelError_ThenDuplicateEmailMapped(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/inbounds/addClient", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, false, "Duplicate email: a@vpn.id", nil)
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.AddClient(ctx, xui.AddClientPayload{InboundID: 1, Client: xui.ClientSpec{Email: "a@vpn.id"}})
	if err == nil {
		t.Fatal("expected error")
	}
	xe, ok := err.(*xui.XUIError)
	if !ok {
		t.Fatalf("error type = %T, want *XUIError", err)
	}
	if xe.Code != xui.CodeDuplicateEmail {
		t.Errorf("code = %s, want DUPLICATE_EMAIL (msg: %s)", xe.Code, xe.Message)
	}
}

func TestGetServerStatus_ThenParsed(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/server/status", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, true, "", xui.Status{CPU: 12.5, CPUCount: 2, Uptime: 999})
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := c.GetServerStatus(ctx)
	if err != nil {
		t.Fatalf("GetServerStatus: %v", err)
	}
	if st.CPU != 12.5 || st.CPUCount != 2 || st.Uptime != 999 {
		t.Errorf("status = %+v", st)
	}
}
