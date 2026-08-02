//go:build windows

package update

import (
	"strings"
	"testing"
)

func TestHelperScriptParamBlockFirst(t *testing.T) {
	trimmed := strings.TrimLeft(helperScript, " \t\r\n")
	if !strings.HasPrefix(trimmed, "param(") {
		t.Fatalf("helper script must declare param() before any other statements")
	}
}

func TestHelperScriptUsesNamedArguments(t *testing.T) {
	for _, name := range []string{"$AppPid", "$TargetDir", "$StagedDir"} {
		if !strings.Contains(helperScript, name) {
			t.Fatalf("helper script missing parameter %s", name)
		}
	}
}

func TestHelperScriptCopiesRecursively(t *testing.T) {
	// Subdirectories in the staged payload must be mirrored, not skipped.
	if !strings.Contains(helperScript, "-Recurse") {
		t.Fatalf("helper script must copy staged subdirectories with -Recurse")
	}
	if strings.Contains(helperScript, "Get-ChildItem -LiteralPath $StagedDir -File") {
		t.Fatalf("helper script still uses top-level-only -File enumeration")
	}
}

func TestHelperScriptValidatesStagedBinary(t *testing.T) {
	// The target install must not be overwritten unless the new GUI binary is staged.
	if !strings.Contains(helperScript, "svpchain-gui.exe") {
		t.Fatalf("helper script missing svpchain-gui.exe reference")
	}
	if !strings.Contains(helperScript, "$stagedExe") {
		t.Fatalf("helper script must verify the staged svpchain-gui.exe before copying")
	}
}
