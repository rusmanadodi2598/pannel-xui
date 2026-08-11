// Package xui also hosts the endpoint methods.
//
// @file      internal/repository/xui/endpoints.go
// @for       GetInbounds, client CRUD, traffic, onlines, server status, restart.
// @uses      context, net/url, encoding/json
// @reason    Route paths & payloads verified from web/controller/api.go + web/service/inbound.go.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// GetInbounds lists all inbounds of the panel (GET /xui/API/inbounds/).
func (c *Client) GetInbounds(ctx context.Context) ([]Inbound, error) {
	var out []Inbound
	if err := c.do(ctx, "GET", "xui/API/inbounds/", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddClientPayload is the form data for addClient: inbound id + settings JSON.
type AddClientPayload struct {
	InboundID int
	Client    ClientSpec
}

// AddClient creates a client on an inbound (POST /xui/API/inbounds/addClient).
// Form: id=<inboundId>&settings={"clients":[...]} — verified from InboundController.
func (c *Client) AddClient(ctx context.Context, payload AddClientPayload) error {
	settings, err := clientsSettingsJSON([]ClientSpec{payload.Client})
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("id", fmt.Sprintf("%d", payload.InboundID))
	form.Set("settings", settings)
	return c.do(ctx, "POST", "xui/API/inbounds/addClient", form, nil)
}

// UpdateClientPayload is the form data for updateClient.
type UpdateClientPayload struct {
	InboundID int
	ClientID  string
	Client    ClientSpec
}

// UpdateClient updates a client (POST /xui/API/inbounds/updateClient/:clientId).
func (c *Client) UpdateClient(ctx context.Context, payload UpdateClientPayload) error {
	settings, err := clientsSettingsJSON([]ClientSpec{payload.Client})
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("id", fmt.Sprintf("%d", payload.InboundID))
	form.Set("settings", settings)
	path := fmt.Sprintf("xui/API/inbounds/updateClient/%s", url.PathEscape(payload.ClientID))
	return c.do(ctx, "POST", path, form, nil)
}

// DeleteClient removes a client (POST /xui/API/inbounds/:id/delClient/:clientId).
func (c *Client) DeleteClient(ctx context.Context, inboundID int, clientID string) error {
	path := fmt.Sprintf("xui/API/inbounds/%d/delClient/%s", inboundID, url.PathEscape(clientID))
	return c.do(ctx, "POST", path, nil, nil)
}

// GetClientTrafficByEmail returns one client traffic by email
// (GET /xui/API/inbounds/getClientTraffics/:email) — obj is a single object.
func (c *Client) GetClientTrafficByEmail(ctx context.Context, email string) (ClientTraffic, error) {
	var out ClientTraffic
	path := fmt.Sprintf("xui/API/inbounds/getClientTraffics/%s", url.PathEscape(email))
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return ClientTraffic{}, err
	}
	return out, nil
}

// GetClientTrafficByID returns client traffic list by UUID/id
// (GET /xui/API/inbounds/getClientTrafficsById/:id) — obj is an ARRAY here.
func (c *Client) GetClientTrafficByID(ctx context.Context, id string) ([]ClientTraffic, error) {
	var out []ClientTraffic
	path := fmt.Sprintf("xui/API/inbounds/getClientTrafficsById/%s", url.PathEscape(id))
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOnlineClients lists online users (POST /xui/API/inbounds/onlines).
func (c *Client) GetOnlineClients(ctx context.Context) ([]OnlineUser, error) {
	var out []OnlineUser
	if err := c.do(ctx, "POST", "xui/API/inbounds/onlines", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetServerStatus returns panel health (GET /xui/API/server/status).
func (c *Client) GetServerStatus(ctx context.Context) (Status, error) {
	var out Status
	if err := c.do(ctx, "GET", "xui/API/server/status", nil, &out); err != nil {
		return Status{}, err
	}
	return out, nil
}

// RestartXray restarts the panel's xray service (POST /xui/API/server/restartXrayService).
func (c *Client) RestartXray(ctx context.Context) error {
	return c.do(ctx, "POST", "xui/API/server/restartXrayService", nil, nil)
}

// clientsSettingsJSON builds the {"clients":[...]} settings string for form posts.
func clientsSettingsJSON(clients []ClientSpec) (string, error) {
	raw, err := json.Marshal(map[string][]ClientSpec{"clients": clients})
	if err != nil {
		return "", fmt.Errorf("xui marshal settings: %w", err)
	}
	return string(raw), nil
}
