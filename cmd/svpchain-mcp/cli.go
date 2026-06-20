package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/99designs/keyring"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/term"

	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	signermcp "github.com/svpchain/svpchain-agent/internal/mcp"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

func usage(w io.Writer) {
	fmt.Fprint(w, `svpchain-mcp — local stdio signer MCP server for svpchain

Usage:
  svpchain-mcp --chain-id <id>          serve (default)
  svpchain-mcp import --chain-id <id>   store a key in the OS credential store
  svpchain-mcp delete --chain-id <id>   remove a stored key
  svpchain-mcp list                     list stored chain ids
  svpchain-mcp help                     show this help

One key per chain: the signing key is read from the OS credential store
(macOS Keychain, Windows Credential Manager, Linux Secret Service) under the
chain id, falling back to the SIGNER_KEY_HEX env var for headless use.

Prefer the svpchain-gui app for a graphical setup experience.

Run a subcommand with -h for its flags, e.g. 'svpchain-mcp import -h'.
`)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	chainID := fs.String("chain-id", "", "chain id this signer is bound to: it refuses any payload or challenge for a different chain and selects the stored key of the same name (required)")
	evmChainID := fs.Uint64("evm-chain-id", 0, "numeric EIP-155 chain id for sign_evm_transaction; defaults to the number parsed from --chain-id (e.g. svp_2517-1 or svp-2517-1 => 2517)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chainID == "" {
		return fmt.Errorf("--chain-id is required")
	}

	evmID := *evmChainID
	if evmID == 0 {
		// Best-effort: parse the EIP-155 number out of the chain id
		// (name<sep><number>-<epoch>, sep "_" or "-"). If --chain-id doesn't
		// follow that shape, EVM signing stays disabled until the operator
		// supplies --evm-chain-id; Cosmos signing is unaffected.
		if parsed, ok := parseEVMChainID(*chainID); ok {
			evmID = parsed
		} else {
			fmt.Fprintf(os.Stderr, "svpchain-mcp: could not derive an EVM chain id from --chain-id %q; "+
				"sign_evm_transaction is disabled until you pass --evm-chain-id\n", *chainID)
		}
	}

	hexKey, source, err := acquireKey(*chainID)
	if err != nil {
		return err
	}
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return fmt.Errorf("parse key from %s: %w", source, err)
	}
	fmt.Fprintf(os.Stderr, "svpchain-mcp: loaded signing key from %s\n", source)

	h := &signermcp.Handlers{Priv: priv, ChainID: *chainID, EVMChainID: evmID}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "svpchain-agent",
		Version: "v0.1.0",
	}, nil)
	signermcp.Register(srv, h)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx, &mcp.StdioTransport{})
}

// parseEVMChainID extracts the numeric EIP-155 chain id from a cosmos chain id
// of the form name<sep><number>-<epoch>, where <sep> is "_" (evmos-style, e.g.
// svp_2517-1) or "-" (older-style, e.g. svp-2517-1) — both yield 2517. It
// strips the trailing "-<epoch>" and reads the trailing run of digits; it
// returns ok=false when there's no epoch dash or no trailing number, so the
// caller can fall back to an explicit --evm-chain-id.
func parseEVMChainID(cosmosChainID string) (uint64, bool) {
	// Drop the trailing -<epoch>; both forms carry it.
	dash := strings.LastIndex(cosmosChainID, "-")
	if dash < 0 {
		return 0, false
	}
	head := cosmosChainID[:dash]
	// The chain number is the trailing digit run of head, after the name<sep>
	// prefix (the name may itself contain digits, so scan only the suffix).
	i := len(head)
	for i > 0 && head[i-1] >= '0' && head[i-1] <= '9' {
		i--
	}
	id, err := strconv.ParseUint(head[i:], 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func acquireKey(name string) (hexKey, source string, err error) {
	var ring keyring.Keyring
	if r, openErr := keystore.Open(); openErr == nil {
		ring = r
	}
	return manage.SelectKey(ring, name, os.Getenv("SIGNER_KEY_HEX"))
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	chainID := fs.String("chain-id", "", "chain id this key signs for; also the name it's stored under (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chainID == "" {
		return fmt.Errorf("--chain-id is required")
	}

	hexKey, err := readKeyForImport()
	if err != nil {
		return err
	}

	res, err := manage.Import(*chainID, hexKey)
	if err != nil {
		return err
	}
	if len(res.Conflicts) > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: key %s is already stored under %s; reusing a key across chains is discouraged\n",
			res.Owner, strings.Join(res.Conflicts, ", "))
	}
	fmt.Fprintf(os.Stderr, "Stored key for %s (%s)\n", res.Owner, *chainID)
	return nil
}

func readKeyForImport() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return readKeyFrom(os.Stdin)
	}
	if env := strings.TrimSpace(os.Getenv("SIGNER_KEY_HEX")); env != "" {
		return env, nil
	}
	fmt.Fprint(os.Stderr, "Enter private key (hidden): ")
	bz, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read key from prompt: %w", err)
	}
	key := strings.TrimSpace(string(bz))
	if key == "" {
		return "", fmt.Errorf("no key entered")
	}
	return key, nil
}

func readKeyFrom(r io.Reader) (string, error) {
	bz, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read key: %w", err)
	}
	key := strings.TrimSpace(string(bz))
	if key == "" {
		return "", fmt.Errorf("no key provided")
	}
	return key, nil
}

func runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	chainID := fs.String("chain-id", "", "chain id of the key to delete (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *chainID == "" {
		return fmt.Errorf("--chain-id is required")
	}
	if err := manage.Delete(*chainID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted key %q\n", *chainID)
	return nil
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, err := manage.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Println(e.ChainID)
	}
	return nil
}
