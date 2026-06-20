// Command svpchain-mcp is svpchain's local signing MCP service —
// the signing side of a dual-MCP architecture; the other side is a remote build + broadcast MCP service.
//
// Runs over stdio (no network port; the agent process that starts it is the trust boundary).
// MCP tool handlers live in internal/mcp; key management in internal/manage.
package main
