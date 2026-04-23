// Command example-github is a reference plugin for bitgit.
//
// It speaks JSON-RPC 2.0 over stdin/stdout. For every hook bitgit dispatches,
// it prints a brief log line to stderr and approves the operation. Use it as
// a copy-paste starting point for real plugins.
//
// THIS IS NOT FOR PRODUCTION USE. It exists to document the protocol.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcEr `json:"error,omitempty"`
}

type rpcEr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type hookResult struct {
	Allow  bool           `json:"allow"`
	Reason string         `json:"reason,omitempty"`
	Mutate map[string]any `json:"mutate,omitempty"`
}

func main() {
	in := bufio.NewReader(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(in)
	for {
		var req rpcReq
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintln(os.Stderr, "example-github: decode error:", err)
			return
		}
		fmt.Fprintf(os.Stderr, "example-github: got hook %s\n", req.Method)
		resp := rpcResp{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: hookResult{
				Allow:  true,
				Reason: "example plugin always approves",
			},
		}
		if err := out.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "example-github: encode error:", err)
			return
		}
	}
}
