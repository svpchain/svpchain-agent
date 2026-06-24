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
