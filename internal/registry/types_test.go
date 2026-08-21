package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTxResult_AllEvents(t *testing.T) {
	top := TxResult{
		Events: []AbciEvent{{Type: "transfer", Attributes: []AbciAttribute{{Key: "recipient", Value: "svp1top"}}}},
		Logs: []TxLog{{
			Events: []AbciEvent{{Type: "transfer", Attributes: []AbciAttribute{{Key: "recipient", Value: "svp1log"}}}},
		}},
	}
	require.Equal(t, "svp1top", top.AllEvents()[0].Attributes[0].Value)

	fromLogs := TxResult{
		Logs: []TxLog{{
			Events: []AbciEvent{{Type: "transfer", Attributes: []AbciAttribute{{Key: "recipient", Value: "svp1log"}}}},
		}},
	}
	require.Equal(t, "svp1log", fromLogs.AllEvents()[0].Attributes[0].Value)

	require.Empty(t, TxResult{}.AllEvents())
}
