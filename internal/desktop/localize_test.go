package desktop

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/i18n"
	"github.com/svpchain/svpchain-agent/internal/manage"
)

func TestLocalized_NoSigningKey(t *testing.T) {
	i18n.SetLang(i18n.Zh)
	err := localized(&manage.NoSigningKeyError{ChainID: "svp-2517-1"})
	require.Contains(t, err.Error(), "密钥")

	i18n.SetLang(i18n.En)
	err = localized(&manage.NoSigningKeyError{ChainID: "svp-2517-1"})
	require.Contains(t, err.Error(), "Keys tab")
}

func TestLocalized_AgentBusy(t *testing.T) {
	i18n.SetLang(i18n.Zh)
	require.Contains(t, localized(i18n.ErrAgentBusy).Error(), "助手")

	i18n.SetLang(i18n.En)
	require.Contains(t, localized(i18n.ErrAgentBusy).Error(), "Assistant")
}

func TestLocalized_UnknownPassthrough(t *testing.T) {
	raw := errors.New("remote mcp: connection refused")
	i18n.SetLang(i18n.Zh)
	require.Contains(t, localized(raw).Error(), "远程 MCP")
}
