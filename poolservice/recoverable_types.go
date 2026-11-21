// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

const Version3 uint32 = 3
const recoverablePathPrefix = "/pool/v3/"
const MaxRecoverableClientConfigSize = 65536 + 1024

type RecoverableDailyClientConfig struct {
	Version      uint32                `json:"version"`
	Base         DailyClientConfig     `json:"base"`
	IssueProfile registry.IssueProfile `json:"issue_profile"`
}

// RecoverableDailySelection is public correlation only, never a credential.
type RecoverableDailySelection struct {
	Version              uint32
	BaseVersion          uint32
	URL                  string
	PublicKey            string
	Source               poolshare.Config
	IssueProfile         registry.IssueProfile
	SettlementConfigID   string
	RecipientScript      string
	BaseTransportTrustID string
	TransportTrustID     string
}
type recoverableClientBinding struct {
	selection RecoverableDailySelection
	profileID string
}
type recoverableServerBinding struct {
	profile   registry.IssueProfile
	profileID string
}
type IssueRequest struct {
	RequestID string `json:"request_id"`
}
type issueRequest = IssueRequest

// IssueResponse's private provenance is intentionally absent from all wire and
// stored bytes. Parsing a response cannot manufacture a client receipt.
type IssueResponse struct {
	Version            uint32                `json:"version"`
	BindingID          string                `json:"binding_id"`
	SettlementConfigID string                `json:"settlement_config_id"`
	RecipientScript    string                `json:"recipient_script"`
	IssueProfileID     string                `json:"issue_profile_id"`
	RequestID          string                `json:"request_id"`
	GymID              string                `json:"gym_id"`
	PublicKey          string                `json:"public_key"`
	Result             poolshare.IssueResult `json:"result"`
	provenance         *issueResponseProvenance
}
type issueResponseProvenance struct {
	client  *Client
	request string
	digest  [32]byte
	size    int
}
type RecoverableWorkResponse struct {
	Version            uint32               `json:"version"`
	BindingID          string               `json:"binding_id"`
	SettlementConfigID string               `json:"settlement_config_id"`
	RecipientScript    string               `json:"recipient_script"`
	IssueProfileID     string               `json:"issue_profile_id"`
	Work               poolshare.MiningWork `json:"work"`
}
type RecoverableSubmissionResponse struct {
	Version            uint32               `json:"version"`
	BindingID          string               `json:"binding_id"`
	SettlementConfigID string               `json:"settlement_config_id"`
	RecipientScript    string               `json:"recipient_script"`
	IssueProfileID     string               `json:"issue_profile_id"`
	Submission         poolshare.Submission `json:"submission"`
}
type RecoverableBalanceResponse struct {
	Version            uint32                  `json:"version"`
	BindingID          string                  `json:"binding_id"`
	SettlementConfigID string                  `json:"settlement_config_id"`
	RecipientScript    string                  `json:"recipient_script"`
	IssueProfileID     string                  `json:"issue_profile_id"`
	Balance            poolsettle.DailyBalance `json:"balance"`
	ObservedAtMS       int64                   `json:"observed_at_ms"`
}
type RecoverablePayoutPage struct {
	Version            uint32                       `json:"version"`
	BindingID          string                       `json:"binding_id"`
	SettlementConfigID string                       `json:"settlement_config_id"`
	IssueProfileID     string                       `json:"issue_profile_id"`
	Revision           uint64                       `json:"revision,string"`
	HighBatch          uint64                       `json:"high_batch,string"`
	BeforeBatch        uint64                       `json:"before_batch,string"`
	NextBefore         uint64                       `json:"next_before,string"`
	Done               bool                         `json:"done"`
	RecipientScript    string                       `json:"recipient_script"`
	Balance            poolsettle.DailyBalance      `json:"balance"`
	ObservedAtMS       int64                        `json:"observed_at_ms"`
	Items              []poolsettle.DailyPayoutItem `json:"items"`
}

func (p RecoverablePayoutPage) daily() poolsettle.DailyPayoutPage {
	return poolsettle.DailyPayoutPage{Version: Version2, BindingID: p.BindingID, SettlementConfigID: p.SettlementConfigID, Revision: p.Revision, HighBatch: p.HighBatch, BeforeBatch: p.BeforeBatch, NextBefore: p.NextBefore, Done: p.Done, RecipientScript: p.RecipientScript, Balance: p.Balance, ObservedAtMS: p.ObservedAtMS, Items: p.Items}
}
func recoverablePage(p poolsettle.DailyPayoutPage, profile string) RecoverablePayoutPage {
	return RecoverablePayoutPage{Version3, p.BindingID, p.SettlementConfigID, profile, p.Revision, p.HighBatch, p.BeforeBatch, p.NextBefore, p.Done, p.RecipientScript, p.Balance, p.ObservedAtMS, p.Items}
}
