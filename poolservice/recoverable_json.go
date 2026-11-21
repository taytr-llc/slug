// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import "github.com/taytr-llc/slug/poolsettle"

func decodeRecoverableIssue(raw []byte, v *IssueResponse) error {
	return decodeSelectedWork(raw, MaxWorkResponseSize, v, "result.work")
}

var recoverableBalanceFields = func() []payoutField {
	f := append([]payoutField{}, dailyMoneyFields...)
	f = append(f, payoutField{name: "issue_profile_id", kind: 's', limit: 64})
	return append(f, dailyBalanceResponseFields[len(dailyMoneyFields):]...)
}()
var recoverablePageFields = func() []payoutField {
	f := append([]payoutField{}, dailyPageFields[:3]...)
	f = append(f, payoutField{name: "issue_profile_id", kind: 's', limit: 64})
	return append(f, dailyPageFields[3:]...)
}()

func decodeRecoverableBalance(raw []byte, v *RecoverableBalanceResponse) error {
	return decodeDailyValue(raw, MaxResponseSize, recoverableBalanceFields, v)
}
func decodeRecoverablePage(raw []byte, v *RecoverablePayoutPage) error {
	return decodeDailyValue(raw, MaxPayoutResponseSize, recoverablePageFields, v)
}

// Existing public money projections retain their version2 in-process shape.
// The selected v3 envelope is completely checked before that projection.
func (c *Client) decodeSelectedWorkResponse(raw []byte, out *DailyWorkResponse) error {
	if c.recoverable == nil {
		return decodeDailyStrict(raw, MaxWorkResponseSize, out)
	}
	var r RecoverableWorkResponse
	if decodeDailyStrict(raw, MaxWorkResponseSize, &r) != nil || !c.recoverableBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.IssueProfileID) {
		return errConfig
	}
	*out = DailyWorkResponse{Version2, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.Work}
	return nil
}
func (c *Client) decodeSelectedSubmissionResponse(raw []byte, out *DailySubmissionResponse) error {
	if c.recoverable == nil {
		return decodeDailyStrict(raw, MaxResponseSize, out)
	}
	var r RecoverableSubmissionResponse
	if decodeDailyStrict(raw, MaxResponseSize, &r) != nil || !c.recoverableBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.IssueProfileID) {
		return errConfig
	}
	*out = DailySubmissionResponse{Version2, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.Submission}
	return nil
}
func (c *Client) decodeSelectedBalance(raw []byte, out *DailyBalanceResponse) error {
	if c.recoverable == nil {
		return decodeDailyBalance(raw, out)
	}
	var r RecoverableBalanceResponse
	if decodeRecoverableBalance(raw, &r) != nil || !c.recoverableBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.IssueProfileID) {
		return errConfig
	}
	*out = DailyBalanceResponse{Version2, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.Balance, r.ObservedAtMS}
	return nil
}
func (c *Client) decodeSelectedPage(raw []byte, out *poolsettle.DailyPayoutPage) error {
	if c.recoverable == nil {
		return decodeDailyPage(raw, out)
	}
	var r RecoverablePayoutPage
	if decodeRecoverablePage(raw, &r) != nil || !c.recoverableBinding(r.Version, r.BindingID, r.SettlementConfigID, r.RecipientScript, r.IssueProfileID) {
		return errConfig
	}
	*out = r.daily()
	return nil
}
