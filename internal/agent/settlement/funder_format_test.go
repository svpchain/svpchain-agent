package settlement

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatTokenAmount(t *testing.T) {
	for _, tc := range []struct {
		name     string
		amount   string
		decimals uint8
		want     string
	}{
		{name: "one USDC", amount: "1000000", decimals: 6, want: "1"},
		{name: "one cent USDC", amount: "10000", decimals: 6, want: "0.01"},
		{name: "trims insignificant zeros", amount: "1234500", decimals: 6, want: "1.2345"},
		{name: "zero", amount: "0", decimals: 6, want: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatTokenAmount(tc.amount, tc.decimals)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
