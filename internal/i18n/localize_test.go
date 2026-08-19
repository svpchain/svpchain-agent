package i18n

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/manage"
)

func TestFormatNoSigningKey(t *testing.T) {
	SetLang(En)
	msg := FormatNoSigningKey("svp-2517-1")
	require.Contains(t, msg, "svp-2517-1")
	require.Contains(t, msg, "Keys tab")

	SetLang(Zh)
	msg = FormatNoSigningKey("svp-2517-1")
	require.Contains(t, msg, "svp-2517-1")
	require.Contains(t, msg, "密钥")
}

func TestLocalize_ManageErrors(t *testing.T) {
	SetLang(Zh)
	require.Contains(t, Localize(fmt.Errorf("invalid key: hex decode: foo")), "私钥无效")
	require.Contains(t, Localize(fmt.Errorf("whitelist entry already exists")), "白名单")

	SetLang(En)
	require.Contains(t, Localize(fmt.Errorf("signer binary path is required")), "Signer binary path")
}

func TestLocalize_NoSigningKeyTyped(t *testing.T) {
	SetLang(Zh)
	msg := Localize(&manage.NoSigningKeyError{ChainID: "svp-2517-1"})
	require.Contains(t, msg, "密钥")
}

func TestLocalizeStepTitle(t *testing.T) {
	SetLang(Zh)
	require.Equal(t, "正在启动助手…", LocalizeStepTitle("Starting assistant…"))
	require.Equal(t, "思考中…（第 2 轮）", LocalizeStepTitle("Thinking… (round 2)"))

	SetLang(En)
	require.Equal(t, "Starting assistant…", LocalizeStepTitle("Starting assistant…"))
}

func TestLocalizeAgentAnswer(t *testing.T) {
	SetLang(Zh)
	raw := "Transfer rejected — no whitelist configured for chain \"svp-2517-1\" — add a recipient in the Security tab before transferring. No transaction was built, signed, or broadcast."
	msg := LocalizeAgentAnswer(raw)
	require.Contains(t, msg, "转账被拒绝")
	require.Contains(t, msg, "安全")
}

func TestLocalize_Context(t *testing.T) {
	SetLang(Zh)
	require.Equal(t, "已取消", Localize(context.Canceled))
}
