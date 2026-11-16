// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

// GetCheckpointInfoCmd requests report-only signed league checkpoint state.
type GetCheckpointInfoCmd struct{}

// NewGetCheckpointInfoCmd returns a getcheckpointinfo command.
func NewGetCheckpointInfoCmd() *GetCheckpointInfoCmd {
	return &GetCheckpointInfoCmd{}
}

// SignedCheckpointResult reports an authenticated declaration and its relation
// to one local best-chain snapshot. Status never revokes the signed statement.
type SignedCheckpointResult struct {
	Version     uint32 `json:"version"`
	Network     uint32 `json:"network"`
	Genesis     string `json:"genesis"`
	Height      int32  `json:"height"`
	Hash        string `json:"hash"`
	Signature   string `json:"signature"`
	Status      string `json:"status"`
	LocalHash   string `json:"localhash,omitempty"`
	Descendants int32  `json:"descendants"`
}

// GetCheckpointInfoResult describes the authority and signed declarations.
// Finality is league policy only, never a block acceptance rule.
type GetCheckpointInfoResult struct {
	Mode                string                   `json:"mode"`
	AuthorityConfigured bool                     `json:"authorityconfigured"`
	PublicKey           string                   `json:"publickey,omitempty"`
	RequiredDescendants int32                    `json:"requireddescendants"`
	BestHeight          int32                    `json:"bestheight"`
	BestHash            string                   `json:"besthash"`
	Conflict            bool                     `json:"conflict"`
	Checkpoints         []SignedCheckpointResult `json:"checkpoints"`
}

func init() {
	MustRegisterCmd("getcheckpointinfo", (*GetCheckpointInfoCmd)(nil), 0)
}
