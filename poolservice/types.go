// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolservice transports pinned pool work, share receipts and a gym's
// own historical balance. It exposes no operator accounting or payment action.
package poolservice

import (
	"context"
	"net/http"
	"sync"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"golang.org/x/time/rate"
)

const Version1 uint32 = 1
const Version2 uint32 = 2
const MaxRequestSize = 1024
const MaxWorkResponseSize = prospectwork.MaxTemplateSize + (64 << 10)
const MaxResponseSize = 64 << 10
const pathPrefix = "/pool/v1/"

type ServerConfig struct {
	Version uint32
	Listen  string
}
type ClientConfig struct {
	Version       uint32           `json:"version"`
	URL           string           `json:"url"`
	Token         string           `json:"token"`
	CACertificate string           `json:"ca_certificate"`
	PublicKey     string           `json:"public_key"`
	Source        poolshare.Config `json:"source"`
}
type ClientError struct {
	Code           string
	OutcomeUnknown bool
}

func (e *ClientError) Error() string {
	if e.OutcomeUnknown {
		return "pool service: outcome unknown; recover the same job and nonce"
	}
	return "pool service: " + e.Code
}

type ErrorResponse struct {
	Version uint32   `json:"version"`
	Error   APIError `json:"error"`
}
type APIError struct {
	Code           string `json:"code"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}
type workRequest struct {
	JobID string `json:"job_id"`
}
type submissionRequest struct {
	JobID string `json:"job_id"`
	Nonce uint32 `json:"nonce"`
}
type WorkResponse struct {
	Version   uint32               `json:"version"`
	BindingID string               `json:"binding_id"`
	Work      poolshare.MiningWork `json:"work"`
}
type SubmissionResponse struct {
	Version    uint32               `json:"version"`
	BindingID  string               `json:"binding_id"`
	Submission poolshare.Submission `json:"submission"`
}
type BalanceResponse struct {
	Version      uint32             `json:"version"`
	BindingID    string             `json:"binding_id"`
	Balance      poolsettle.Balance `json:"balance"`
	ObservedAtMS int64              `json:"observed_at_ms"`
}
type clientRates struct{ issue, submit, read *rate.Limiter }
type Server struct {
	recoverable      *recoverableServerBinding
	daily            *dailyServerBinding
	config           ServerConfig
	source           poolshare.Config
	binding          string
	pool             *poolshare.Pool
	settlement       *poolsettle.Store
	sessions         prospectservice.SessionAuthorizer
	active           chan struct{}
	mu               sync.Mutex
	started, closing bool
	handlers         sync.WaitGroup
	rates            map[string]*clientRates
}

// Client's immutable trust/public identity may survive token refresh. Tokens
// stay in memory only; owners replace clients rather than mutate credentials.
type Client struct {
	recoverable         *recoverableClientBinding
	daily               *dailyClientBinding
	config              ClientConfig
	binding, trust, gym string
	http                *http.Client
}

// Authority is the transport boundary for a later durable pool worker. Work
// reconstructs the pool's fixed extra-nonce-zero candidate, not a local payout.
type Authority interface {
	ProtocolVersion() uint32
	AuthorityOrigin() string
	SourceConfig() poolshare.Config
	TransportTrustID() string
	PublicKey() string
	Issue(context.Context) (poolshare.MiningWork, error)
	Work(context.Context, string) (poolshare.MiningWork, error)
	Submit(context.Context, poolshare.MiningWork, uint32) (poolshare.Submission, error)
	Lookup(context.Context, poolshare.MiningWork, uint32) (poolshare.Submission, error)
	Balance(context.Context) (BalanceResponse, error)
}
