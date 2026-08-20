package hitl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsk_nilDenies(t *testing.T) {
	err := Ask(context.Background(), nil, Request{Kind: KindSignTransaction, Title: "Sign Cosmos transaction"})
	var denied *Denied
	require.ErrorAs(t, err, &denied)
	require.Equal(t, KindSignTransaction, denied.Kind)
	require.Contains(t, denied.StopMessage(), "Signing declined")
}

func TestAsk_falseDenies(t *testing.T) {
	err := Ask(context.Background(), func(context.Context, Request) bool { return false }, Request{
		Kind:  KindDelegateTask,
		Title: "Delegate task",
	})
	var denied *Denied
	require.ErrorAs(t, err, &denied)
	require.False(t, denied.signing())
	require.Contains(t, denied.StopMessage(), "Declined —")
	require.NotContains(t, denied.StopMessage(), "Signing declined")
}

func TestAsk_trueApproves(t *testing.T) {
	require.NoError(t, Ask(context.Background(), func(context.Context, Request) bool { return true }, Request{Kind: KindSignEVM}))
}

func TestAsk_cancelledContextDenies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Ask(ctx, func(context.Context, Request) bool { return true }, Request{Kind: KindSignEVM, Title: "Sign EVM transaction"})
	require.ErrorAs(t, err, new(*Denied))
}

func TestNeedsConfirm(t *testing.T) {
	require.True(t, NeedsConfirm("sign_transaction"))
	require.True(t, NeedsConfirm("sign_evm_transaction"))
	require.True(t, NeedsConfirm("sign_typed_data"))
	require.False(t, NeedsConfirm("sign_challenge"))
	require.False(t, NeedsConfirm("signer_whoami"))
	require.False(t, NeedsConfirm("build_bank_send"))
}

func TestDeniedErrorMatchesStopMessageTitle(t *testing.T) {
	d := &Denied{Kind: KindSignTypedData, Title: "Sign typed data"}
	require.Error(t, d)
	require.True(t, errors.As(d, new(*Denied)))
	require.Contains(t, d.Error(), "Sign typed data")
}
