package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const keyA = "bdac2376052bca8d4ae7c2012578f00ef5c2ce40c06bb0a06f986cd1e016c570"

func TestReadKeyFrom(t *testing.T) {
	got, err := readKeyFrom(strings.NewReader("  " + keyA + "\n"))
	require.NoError(t, err)
	require.Equal(t, keyA, got)

	_, err = readKeyFrom(strings.NewReader("   \n\t "))
	require.Error(t, err)

	_, err = readKeyFrom(strings.NewReader(""))
	require.Error(t, err)
}

func TestParseEVMChainID(t *testing.T) {
	cases := []struct {
		in     string
		wantID uint64
		wantOK bool
	}{
		{"svp_2517-1", 2517, true}, // evmos-style underscore separator
		{"svp-2517-1", 2517, true}, // older-style dash separator
		{"svpchain_1234-1", 1234, true},
		{"cosmos_9001-1", 9001, true},
		{"foo_bar_42-7", 42, true},   // trailing number after the name
		{"localsvp-1", 0, false},     // name has no trailing number
		{"svpchain_-1", 0, false},    // empty number
		{"svpchain_abc-1", 0, false}, // non-numeric
		{"svpchain_1234", 0, false},  // no epoch dash
		{"svpchain_0-1", 0, false},   // zero is not a valid chain id
		{"cosmoshub-4", 0, false},    // non-EVM cosmos id (no chain number)
		{"", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			id, ok := parseEVMChainID(c.in)
			require.Equal(t, c.wantOK, ok)
			require.Equal(t, c.wantID, id)
		})
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	err := run([]string{"bogus"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown subcommand")
}

func TestRun_Help(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		require.NoError(t, run([]string{arg}), "run(%q) should succeed", arg)
	}
}
