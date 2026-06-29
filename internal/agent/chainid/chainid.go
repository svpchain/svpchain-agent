package chainid

import (
	"strconv"
	"strings"
)

// ParseEVM extracts the numeric EIP-155 chain id from a cosmos chain id
// of the form name<sep><number>-<epoch> (e.g. svp-2517-1 → 2517).
func ParseEVM(cosmosChainID string) (uint64, bool) {
	dash := strings.LastIndex(cosmosChainID, "-")
	if dash < 0 {
		return 0, false
	}
	head := cosmosChainID[:dash]
	i := len(head)
	for i > 0 && head[i-1] >= '0' && head[i-1] <= '9' {
		i--
	}
	id, err := strconv.ParseUint(head[i:], 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}
