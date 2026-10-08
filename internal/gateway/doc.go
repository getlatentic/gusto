// Package gateway is the one place an agent reaches tools. A job's harness is given a single MCP server, this one, and the
// gateway calls the real servers: it holds their addresses and credentials, so none of them is in the sandbox; it decides which
// tools the job's caller may use; it writes down each call; and it lets the agent find a tool when it needs one, so that the
// definitions of tools it never uses are not in its context.
//
// It speaks the Model Context Protocol over Streamable HTTP to the harness and to each server it calls.
package gateway
