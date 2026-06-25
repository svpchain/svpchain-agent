package a2aserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// StartServer binds and serves the A2A JSON-RPC endpoint and Agent Card.
func StartServer(ctx context.Context, cfg ServerConfig) error {
	card := BuildAgentCard(cfg.PublicURL)
	executor := NewExecutor(cfg)
	handler := a2asrv.NewHandler(executor)

	mux := http.NewServeMux()
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(handler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddr, err)
	}

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	fmt.Fprintf(os.Stderr, "svpchain-a2a: listening on %s (chain %s)\n", cfg.ListenAddr, cfg.ChainID)
	fmt.Fprintf(os.Stderr, "svpchain-a2a: agent card at %s%s\n", cfg.PublicURL, a2asrv.WellKnownAgentCardPath)
	fmt.Fprintf(os.Stderr, "svpchain-a2a: JSON-RPC at %s/invoke\n", cfg.PublicURL)

	err = srv.Serve(ln)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
