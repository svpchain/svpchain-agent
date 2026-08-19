package skills

import (
	"strings"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func loadSkillsConfigBaseFromPrefs() string {
	return strings.TrimSpace(prefs.Read().SkillsConfigBase)
}
