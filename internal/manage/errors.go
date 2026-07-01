package manage

import (
	"errors"
	"fmt"

	"github.com/svpchain/svpchain-agent/internal/brand"
)

// ErrNoSigningKey is returned when no signing key exists for the requested chain id.
var ErrNoSigningKey = errors.New("no signing key")

// NoSigningKeyError carries the chain id for localized GUI messages.
// Error() returns English text for CLI and logging.
type NoSigningKeyError struct {
	ChainID string
}

func (e *NoSigningKeyError) Error() string {
	return fmt.Sprintf(
		"no signing key for %q: open %s, go to the Keys tab, select Chain ID %q, "+
			"import a private key or use Auto-generate to save it to the OS credential store; "+
			"for headless use, set SIGNER_KEY_HEX",
		e.ChainID, brand.AppDisplayName, e.ChainID,
	)
}

func (e *NoSigningKeyError) Is(target error) bool {
	return target == ErrNoSigningKey
}
