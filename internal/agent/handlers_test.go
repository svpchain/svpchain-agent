package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
)

func TestHandlers_remoteIsLastCatchAll(t *testing.T) {
	hs := dispatchEnv{}.handlers()
	require.NotEmpty(t, hs)
	require.IsType(t, remoteHandler{}, hs[len(hs)-1])
	require.True(t, hs[len(hs)-1].Match("build_bank_send"))
	require.True(t, hs[len(hs)-1].Match("anything_unknown"))
}

func TestHandlers_firstMatchWins(t *testing.T) {
	env := dispatchEnv{}
	cases := []struct {
		name string
		want string
	}{
		{"http_fetch", "httpHandler"},
		{"x402_prepare_typed_data", "x402Handler"},
		{"x402_build_payment", "x402Handler"},
		{"a2a_send_message", "a2aHandler"},
		{ConnectTool, "connectHandler"},
		{"search_agents", "discoverHandler"},
		{skills.ReferenceToolName, "skillRefHandler"},
		{"sign_transaction", "localHandler"},
		{"sign_evm_transaction", "localHandler"},
		{"sign_typed_data", "localHandler"},
		{"sign_challenge", "localHandler"},
		{"signer_whoami", "localHandler"},
		{"evm_to_bech32", "localHandler"},
		{"build_bank_send", "remoteHandler"},
		{"broadcast_signed_tx", "remoteHandler"},
		{"whoami", "remoteHandler"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstHandlerName(env, tc.name)
			require.Equal(t, tc.want, got, "local sign_* must never match remoteHandler")
		})
	}
}

func TestResumePaidToolRequiresSavedPublishedCapability(t *testing.T) {
	env := dispatchEnv{
		paid:           &paidAgentFlow{},
		resumeAgentURL: "https://agent.example",
		resumeAgentTools: map[string]struct{}{
			"build_example": {},
		},
	}
	require.True(t, env.canResumePaidTool("build_example"))
	require.False(t, env.canResumePaidTool("invented_tool"))
	require.False(t, dispatchEnv{paid: &paidAgentFlow{}, resumeAgentURL: "https://agent.example"}.canResumePaidTool("build_example"))
}

func firstHandlerName(env dispatchEnv, name string) string {
	for _, h := range env.handlers() {
		if h.Match(name) {
			return strings.TrimPrefix(fmt.Sprintf("%T", h), "agent.")
		}
	}
	return ""
}
