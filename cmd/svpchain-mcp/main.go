package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	err := run(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "svpchain-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			usage(os.Stdout)
			return nil
		}
	}

	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return runServe(args)
	case "import":
		return runImport(args)
	case "delete", "remove":
		return runDelete(args)
	case "list":
		return runList(args)
	case "a2a":
		return runA2A(args)
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown subcommand %q (want: serve, import, delete, list, a2a)", cmd)
	}
}
