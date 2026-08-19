// Package httphandler also hosts the /api/v1/servers admin handlers (PRD §26.5).
//
// @file      internal/handler/http/servers_api.go
// @for       Server CRUD + per-server health: list, create, get, patch, delete.
// @uses      encoding/json, errors, net/http, strconv, internal/repository/postgres,
// internal/service/server
// @reason    The panel admin surface over HTTP — credential-free reads, sealed
// password on write, guarded delete (AGENTS.md §1.3/§1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// serverDTO is the credential-free server response shape (never password/username).
type serverDTO struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	CountryCode    string   `json:"countryCode"`
	FlagEmoji      string   `json:"flagEmoji"`
	Location       string   `json:"location"`
	IsActive       bool     `json:"isActive"`
	IsOpen         bool     `json:"isOpen"`
	HealthStatus   string   `json:"healthStatus"`
	CurrentClients int      `json:"currentClients"`
	Protocols      []string `json:"protocols"`
}

func toServerDTO(v postgres.ServerAdminView) serverDTO {
	var protocols []string
	_ = json.Unmarshal([]byte(v.Protocols), &protocols)
	if protocols == nil {
		protocols = []string{}
	}
	return serverDTO{
		ID:             v.ID,
		Name:           v.Name,
		Host:           v.Host,
		Port:           v.Port,
		CountryCode:    v.CountryCode,
		FlagEmoji:      v.FlagEmoji,
		Location:       v.Location,
		IsActive:       v.IsActive,
		IsOpen:         v.IsOpen,
		HealthStatus:   v.HealthStatus,
		CurrentClients: v.CurrentClients,
		Protocols:      protocols,
	}
}

// createServerRequest is the POST /servers body (password sealed server-side).
type createServerRequest struct {
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	APIPath     string   `json:"apiPath"`
	UseSSL      bool     `json:"useSsl"`
	CountryCode string   `json:"countryCode"`
	FlagEmoji   string   `json:"flagEmoji"`
	Location    string   `json:"location"`
	Protocols   []string `json:"protocols"`
}

// updateServerRequest is the PATCH /servers/{id} body (pointer = change field).
type updateServerRequest struct {
	Name        *string `json:"name"`
	Host        *string `json:"host"`
	Port        *int    `json:"port"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	APIPath     *string `json:"apiPath"`
	UseSSL      *bool   `json:"useSsl"`
	CountryCode *string `json:"countryCode"`
	FlagEmoji   *string `json:"flagEmoji"`
	Location    *string `json:"location"`
	IsActive    *bool   `json:"isActive"`
	IsOpen      *bool   `json:"isOpen"`
}

func (o Options) listServers(w http.ResponseWriter, r *http.Request) {
	rows, err := o.Servers.ListAll(r.Context())
	if err != nil {
		o.Logger.Error("api: listing servers", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to list servers")
		return
	}
	out := make([]serverDTO, 0, len(rows))
	for _, v := range rows {
		out = append(out, toServerDTO(v))
	}
	writeList(w, http.StatusOK, out, 1, len(out), int64(len(out)))
}

func (o Options) createServer(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
	if err := decodeBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Malformed request body")
		return
	}
	id, err := o.Servers.AddServer(r.Context(), serversvc.NewServerInput{
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    req.Password,
		APIPath:     req.APIPath,
		UseSSL:      req.UseSSL,
		CountryCode: req.CountryCode,
		FlagEmoji:   req.FlagEmoji,
		Location:    req.Location,
		Protocols:   req.Protocols,
	})
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	writeData(w, http.StatusCreated, map[string]int64{"id": id})
}

func (o Options) getServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid server id")
		return
	}
	v, err := o.Servers.GetAdminByID(r.Context(), id)
	if errors.Is(err, postgres.ErrServerNotFound) {
		writeAPIError(w, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found")
		return
	}
	if err != nil {
		o.Logger.Error("api: getting server", "id", id, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to get server")
		return
	}
	writeData(w, http.StatusOK, toServerDTO(v))
}

func (o Options) updateServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid server id")
		return
	}
	var req updateServerRequest
	if err := decodeBody(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Malformed request body")
		return
	}
	upErr := o.Servers.UpdateServer(r.Context(), serversvc.UpdateServerInput{
		ID:          id,
		Name:        req.Name,
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    req.Password,
		APIPath:     req.APIPath,
		UseSSL:      req.UseSSL,
		CountryCode: req.CountryCode,
		FlagEmoji:   req.FlagEmoji,
		Location:    req.Location,
		IsActive:    req.IsActive,
		IsOpen:      req.IsOpen,
	})
	switch {
	case errors.Is(upErr, postgres.ErrServerNotFound):
		writeAPIError(w, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found")
	case upErr != nil:
		writeAPIError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", upErr.Error())
	default:
		writeData(w, http.StatusOK, map[string]int64{"id": id})
	}
}

func (o Options) deleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid server id")
		return
	}
	delErr := o.Servers.DeleteServer(r.Context(), id)
	switch {
	case errors.Is(delErr, postgres.ErrServerHasClients):
		writeAPIError(w, http.StatusConflict, "SERVER_HAS_CLIENTS", "Server has clients; deactivate it instead")
	case errors.Is(delErr, postgres.ErrServerNotFound):
		writeAPIError(w, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found")
	case delErr != nil:
		o.Logger.Error("api: deleting server", "id", id, "error", delErr)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to delete server")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (o Options) serverHealth(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid server id")
		return
	}
	status, err := o.Servers.CheckHealth(r.Context(), id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"serverId": id, "status": status})
}

func decodeBody(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}
