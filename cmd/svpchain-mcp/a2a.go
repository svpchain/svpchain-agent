package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	svpa2aserver "github.com/svpchain/svpchain-agent/internal/a2aserver"
)

func runA2A(args []string) error {
	if len(args) == 0 {
		a2aUsage(os.Stderr)
		return fmt.Errorf("a2a subcommand required (want: serve)")
	}
	switch args[0] {
	case "-h", "--help", "help":
		a2aUsage(os.Stdout)
		return nil
	case "serve":
		return runA2AServe(args[1:])
	default:
		a2aUsage(os.Stderr)
		return fmt.Errorf("unknown a2a subcommand %q (want: serve)", args[0])
	}
}

func a2aUsage(w io.Writer) {
	fmt.Fprint(w, `svpchain-mcp a2a — Google A2A protocol server

Usage:
  svpchain-mcp a2a serve [flags]

Flags:
  --chain-id <id>     Cosmos chain id (defaults to agent_chain_id in prefs.json)
  --listen <addr>     Listen address (default :8080)
  --public-url <url>  Public base URL for Agent Card (default http://127.0.0.1<listen>)

The server exposes:
  /.well-known/agent-card.json   Agent Card discovery
  /invoke                        JSON-RPC A2A endpoint

LLM settings (api key, model, base URL) are read from prefs.json.
`)
}

func runA2AServe(args []string) error {
	fs := flag.NewFlagSet("a2a serve", flag.ContinueOnError)
	chainID := fs.String("chain-id", "", "chain id (defaults to prefs agent_chain_id)")
	listen := fs.String("listen", "", "listen address (default :8080)")
	publicURL := fs.String("public-url", "", "public base URL for agent card")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := svpa2aserver.ConfigFromPrefs(*chainID, *listen, *publicURL)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return svpa2aserver.StartServer(ctx, cfg)
}
