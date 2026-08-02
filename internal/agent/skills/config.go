package skills

import (
	"strings"

	"github.com/svpchain/svpchain-local-agent/internal/prefs"
)

func loadSkillsConfigBaseFromPrefs() string {
	return strings.TrimSpace(prefs.Read().SkillsConfigBase)
}
