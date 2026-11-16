// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/json"

	"github.com/taytr-llc/slug/btcjson"
)

// FutureGetCheckpointInfoResult is a promise for signed checkpoint state.
type FutureGetCheckpointInfoResult chan *Response

// Receive waits for the report-only checkpoint result.
func (r FutureGetCheckpointInfoResult) Receive() (*btcjson.GetCheckpointInfoResult, error) {
	data, err := ReceiveFuture(r)
	if err != nil {
		return nil, err
	}
	var result btcjson.GetCheckpointInfoResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetCheckpointInfoAsync requests report-only checkpoint state asynchronously.
func (c *Client) GetCheckpointInfoAsync() FutureGetCheckpointInfoResult {
	return c.SendCmd(btcjson.NewGetCheckpointInfoCmd())
}

// GetCheckpointInfo reports authenticated league declarations, local burial,
// and conflicts without changing chain selection or validation.
func (c *Client) GetCheckpointInfo() (*btcjson.GetCheckpointInfoResult, error) {
	return c.GetCheckpointInfoAsync().Receive()
}
