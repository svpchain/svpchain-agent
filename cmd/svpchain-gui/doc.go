// Command svpchain-gui is the graphical setup tool for svpchain-mcp.
//
// It is a Wails desktop app: the Go application layer lives in internal/desktop
// (key management and in-app updates), and the UI is a Vue frontend embedded
// from frontend/dist. Key and config logic stays in internal/manage.
package main
