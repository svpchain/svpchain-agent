package desktop

import (
	"errors"

	"github.com/svpchain/svpchain-agent/internal/i18n"
)

func localized(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(i18n.Localize(err))
}
