package settlement

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActiveTaskUsable(t *testing.T) {
	full := ActiveTask{
		Endpoint: "https://agent.example",
		TaskID:   "0x" + strings.Repeat("ab", 32),
		Owner:    "0x516c9637B4b26F1f62f553145A9A86F01E890f60",
	}
	require.True(t, full.Usable())

	require.False(t, (*ActiveTask)(nil).Usable())

	noEndpoint := full
	noEndpoint.Endpoint = " "
	require.False(t, noEndpoint.Usable(), "nothing to match a run's agent against")

	shortHash := full
	shortHash.TaskID = "0xabcd"
	require.False(t, shortHash.Usable(), "task_id must be the settlement contract bytes32 id")

	noOwner := full
	noOwner.Owner = ""
	require.False(t, noOwner.Usable(), "the reporter cannot close out a task with no owner")
}
