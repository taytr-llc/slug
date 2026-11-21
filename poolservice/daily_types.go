// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
)

const dailyPathPrefix = "/pool/v2/"

type DailyWorkResponse struct {
	Version            uint32               `json:"version"`
	BindingID          string               `json:"binding_id"`
	SettlementConfigID string               `json:"settlement_config_id"`
	RecipientScript    string               `json:"recipient_script"`
	Work               poolshare.MiningWork `json:"work"`
}
type DailySubmissionResponse struct {
	Version            uint32               `json:"version"`
	BindingID          string               `json:"binding_id"`
	SettlementConfigID string               `json:"settlement_config_id"`
	RecipientScript    string               `json:"recipient_script"`
	Submission         poolshare.Submission `json:"submission"`
}
type DailyBalanceResponse struct {
	Version            uint32                  `json:"version"`
	BindingID          string                  `json:"binding_id"`
	SettlementConfigID string                  `json:"settlement_config_id"`
	RecipientScript    string                  `json:"recipient_script"`
	Balance            poolsettle.DailyBalance `json:"balance"`
	ObservedAtMS       int64                   `json:"observed_at_ms"`
}

func (c *Client) pathPrefix() string {
	if c.daily != nil {
		return dailyPathPrefix
	}
	return pathPrefix
}
func dailyErrorStatus(code string) int {
	if code == "stale_capture" {
		return 409
	}
	return errorStatus(code)
}
func (c *Client) errorStatus(code string) int {
	if c.daily != nil {
		return dailyErrorStatus(code)
	}
	return errorStatus(code)
}
func (c *Client) dailyBinding(version uint32, binding, settlement, recipient string) bool {
	return c != nil && c.daily != nil && version == Version2 && binding == c.binding && settlement == c.daily.settlement && recipient == c.daily.recipient
}
