# Set68 node RPC parameter correction

The frozen adapter document describes the nonverbose transaction read as `getrawtransaction(false)`. That notation was incorrect for this inherited node: `btcjson.GetRawTransactionCmd.Verbose` is `*int` and the RPC server treats nonzero as verbose. The concrete request must use `[txid,0]` for exact raw bytes and `[txid,1]` for verbose location. `getblockheader` still uses a boolean and `sendrawtransaction` keeps its boolean allow-high-fees parameter.

The original frozen contract is preserved. This correction follows inherited `btcjson/chainsvrcmds.go` and `rpcserver.go`, not new runtime output. Independent modeled RPC tests validate requests with the inherited command decoder so a permissive stub cannot conceal a wrong parameter type. No journal bytes, reference transaction, fee, capability or observation meaning changes.
