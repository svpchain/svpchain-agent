package i18n

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectLang_envOverride(t *testing.T) {
	t.Setenv("SVPCHAIN_AGENT_LANG", "zh")
	require.Equal(t, Zh, detectLang())

	t.Setenv("SVPCHAIN_AGENT_LANG", "en")
	require.Equal(t, En, detectLang())
}

func TestDetectLang_langEnv(t *testing.T) {
	t.Setenv("SVPCHAIN_AGENT_LANG", "")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	require.Equal(t, Zh, detectLang())

	t.Setenv("LC_ALL", "en_US.UTF-8")
	require.Equal(t, En, detectLang())
}

func TestIsChineseTag(t *testing.T) {
	require.True(t, isChineseTag("zh-Hans"))
	require.True(t, isChineseTag("zh_CN"))
	require.False(t, isChineseTag("en-US"))
}

func TestInit_catalog(t *testing.T) {
	t.Setenv("SVPCHAIN_AGENT_LANG", "en")
	Init()
	require.Equal(t, "svpchain agent", T().WindowTitle)

	t.Setenv("SVPCHAIN_AGENT_LANG", "zh")
	Init()
	require.Equal(t, "svpchain agent", T().WindowTitle)
}

func TestInitWithPreference(t *testing.T) {
	t.Setenv("SVPCHAIN_AGENT_LANG", "en")
	require.Equal(t, Zh, InitWithPreference("zh"))
	require.Equal(t, En, InitWithPreference("en"))
	require.Equal(t, En, InitWithPreference("invalid"))
}

func TestParseLang(t *testing.T) {
	lang, ok := ParseLang("zh-Hans")
	require.True(t, ok)
	require.Equal(t, Zh, lang)

	lang, ok = ParseLang("english")
	require.True(t, ok)
	require.Equal(t, En, lang)

	_, ok = ParseLang("fr")
	require.False(t, ok)
}

func TestSetLang(t *testing.T) {
	SetLang(En)
	require.Equal(t, En, Current())
	require.Equal(t, "Refresh", T().BtnRefresh)

	SetLang(Zh)
	require.Equal(t, Zh, Current())
	require.Equal(t, "刷新", T().BtnRefresh)
}

func TestDetectLang_defaultEnWithoutChineseEnv(t *testing.T) {
	for _, k := range []string{"SVPCHAIN_AGENT_LANG", "LC_ALL", "LANG", "LC_MESSAGES", "LANGUAGE"} {
		os.Unsetenv(k)
	}
	t.Setenv("LANG", "C")
	require.Equal(t, En, detectLang())
}
