package i18n

import "fmt"

// FormatNoSigningKey returns the localized missing-key guidance for chainID.
func FormatNoSigningKey(chainID string) string {
	return fmt.Sprintf(ErrT().NoSigningKey, chainID, chainID)
}
