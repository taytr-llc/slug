// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"net/http"
)

// NewRecoverableDailyServer selects the original source's exact stored profile
// under its gate. The ledger still binds the unchanged financial Base Config2.
func NewRecoverableDailyServer(ctx context.Context, c ServerConfig, pool *poolshare.Pool, settlement *poolsettle.Store, sessions prospectservice.SessionAuthorizer, profile registry.IssueProfile) (*Server, error) {
	if ctx == nil || pool == nil || c.Version != Version3 {
		return nil, errServer
	}
	return newSelectedDailyServer(ctx, c, pool, settlement, sessions, &profile)
}
func (s *Server) selectedDailyPrefix() string {
	if s != nil && s.recoverable != nil {
		return recoverablePathPrefix
	}
	return dailyPathPrefix
}
func (s *Server) writeSelectedDailyFailure(w http.ResponseWriter, code string) {
	if s == nil || s.recoverable == nil {
		writeDailyFailure(w, code)
		return
	}
	raw, _ := json.Marshal(ErrorResponse{Version3, APIError{code, code == "outcome_unknown"}})
	publicHeaders(w)
	w.WriteHeader(dailyErrorStatus(code))
	_, _ = w.Write(append(raw, '\n'))
}
