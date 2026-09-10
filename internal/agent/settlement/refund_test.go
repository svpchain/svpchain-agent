package settlement

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettlementHash(t *testing.T) {
	valid := "0x" + strings.Repeat("ab", 32)
	hash, err := settlementHash("task_id", valid)
	require.NoError(t, err)
	require.Equal(t, valid, hash.Hex())

	for _, value := range []string{"", "0x1234", "0x" + strings.Repeat("zz", 32)} {
		_, err := settlementHash("task_id", value)
		require.ErrorContains(t, err, "task_id must be a 32-byte")
	}
}

func TestTaskStatusActive(t *testing.T) {
	require.True(t, TaskAssigned.Active())
	require.True(t, TaskBound.Active())
	for _, status := range []TaskStatus{TaskNone, TaskSuccess, TaskFailed, TaskCancelled} {
		require.False(t, status.Active(), status)
	}
}
