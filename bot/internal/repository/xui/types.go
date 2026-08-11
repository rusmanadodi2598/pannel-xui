// Package xui implements the X-UI panel REST client (repository layer).
//
// @file      internal/repository/xui/types.go
// @for       Data types mirroring panel models (Inbound, ClientTraffic, Status, OnlineUser).
// @uses      time
// @reason    Contract verified from this repo's source (web/controller, database/model, xray/) — not the Python reference.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import (
	"encoding/json"
	"time"
)

// ServerConfig holds the connection settings of one X-UI panel instance.
type ServerConfig struct {
	BaseURL    string // scheme://host:port (tanpa trailing slash)
	APIPath    string // basePath panel (default "/"); bisa "/panel/"
	Username   string
	Password   string
	Insecure   bool // skip TLS verify untuk panel self-signed
	Timeout    time.Duration
	ServerID   int64 // untuk key cache session Redis (0 = tanpa cache)
	SessionTTL time.Duration
}

// Inbound mirrors database/model.Inbound (json tags verified from model.go).
type Inbound struct {
	ID             int             `json:"id"`
	Up             int64           `json:"up"`
	Down           int64           `json:"down"`
	Total          int64           `json:"total"`
	Remark         string          `json:"remark"`
	Enable         bool            `json:"enable"`
	ExpiryTime     int64           `json:"expiryTime"`
	ClientStats    []ClientTraffic `json:"clientStats"`
	Listen         string          `json:"listen"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       string          `json:"settings"`
	StreamSettings string          `json:"streamSettings"`
	Tag            string          `json:"tag"`
	Sniffing       string          `json:"sniffing"`
}

// ClientTraffic mirrors xray.ClientTraffic (verified from xray/client_traffic.go).
type ClientTraffic struct {
	ID         int    `json:"id"`
	InboundID  int    `json:"inboundId"`
	Enable     bool   `json:"enable"`
	Email      string `json:"email"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	ExpiryTime int64  `json:"expiryTime"`
	Total      int64  `json:"total"`
	Reset      int    `json:"reset"`
}

// OnlineUser mirrors xray.OnlineUserInfo (verified from xray/api.go).
type OnlineUser struct {
	Email string           `json:"email"`
	IPs   map[string]int64 `json:"ips"`
}

// ClientSpec is a payload client object for addClient/updateClient.
// Per-protocol credential mapping (verified from web/service/inbound.go):
//
//	trojan → Password, shadowsocks → Email (as credential), hysteria → Auth,
//	VLESS/VMess → ID.
type ClientSpec struct {
	ID         string `json:"id,omitempty"`
	Password   string `json:"password,omitempty"`
	Auth       string `json:"auth,omitempty"`
	Email      string `json:"email"`
	LimitIP    int    `json:"limitIp"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	Enable     bool   `json:"enable"`
	Flow       string `json:"flow,omitempty"`
	SubID      string `json:"subId,omitempty"`
	TgID       string `json:"tgId,omitempty"`
	Reset      int    `json:"reset"`
}

// Status mirrors web/service.Status (verified from web/service/server.go).
type Status struct {
	CPU      float64 `json:"cpu"`
	CPUCount int     `json:"cpuCount"`
	Mem      struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"mem"`
	Swap struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"swap"`
	Disk struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"disk"`
	Xray struct {
		State    string `json:"state"`
		ErrorMsg string `json:"errorMsg"`
		Version  string `json:"version"`
	} `json:"xray"`
	Uptime   uint64    `json:"uptime"`
	Loads    []float64 `json:"loads"`
	TCPCount int       `json:"tcpCount"`
	UDPCount int       `json:"udpCount"`
	NetIO    struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
	} `json:"netIO"`
}

// APIResponse is the panel envelope {"success","msg","obj"} (entity.Msg).
// Obj is RawMessage so it can hold any shape; decoded per endpoint.
type APIResponse struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}
