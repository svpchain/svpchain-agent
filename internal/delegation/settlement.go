package delegation

import (
	"context"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	settlementmsgs "github.com/svpchain/svpchain-agent/internal/chainmsgs/settlement"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

// OpenSettlement escrows cap (plus the chain's payment fee, charged on top)
// for one delegated task and returns the landed order. The order id is what a
// task credential's settlement caveat binds to — the paid agent can record
// spend against this order and no other.
func (l *Lifecycle) OpenSettlement(
	ctx context.Context, cap registry.Coin, memo string,
) (registry.Settlement, error) {
	amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(cap.Amount))
	if !ok || !amount.IsPositive() {
		return registry.Settlement{}, fmt.Errorf("cap amount %q is not a positive integer", cap.Amount)
	}
	msg := &settlementmsgs.MsgOpenSettlement{
		Opener: l.Owner(),
		Cap:    sdk.Coin{Denom: strings.TrimSpace(cap.Denom), Amount: amount},
		Memo:   memo,
	}
	if _, err := l.submit(ctx, msg, "/dydxprotocol.settlement.MsgOpenSettlement"); err != nil {
		return registry.Settlement{}, err
	}

	// As with Create: the response id does not surface through REST in a form
	// worth parsing, and the fresh order is unambiguous as the opener's newest.
	orders, err := l.Registry.SettlementsByOpener(ctx, l.Owner())
	if err != nil {
		return registry.Settlement{}, fmt.Errorf("settlement landed but listing it failed: %w", err)
	}
	var newest registry.Settlement
	for _, o := range orders {
		if uint64(o.CreatedAtHeight) >= uint64(newest.CreatedAtHeight) {
			newest = o
		}
	}
	if len(newest.ID) == 0 {
		return registry.Settlement{}, fmt.Errorf("settlement tx succeeded but the order was not found")
	}
	return newest, nil
}

// SettleSettlement closes an order normally: the unrecorded remainder refunds
// to the user, and what agents recorded stays claimable by them.
func (l *Lifecycle) SettleSettlement(ctx context.Context, id []byte) (registry.TxResult, error) {
	return l.submit(ctx, &settlementmsgs.MsgSettle{
		Opener: l.Owner(), SettlementId: id,
	}, "/dydxprotocol.settlement.MsgSettle")
}

// RefundSettlement closes an order by full refund: everything unclaimed comes
// back and outstanding claimables are zeroed — the Emergency-Stop companion.
func (l *Lifecycle) RefundSettlement(ctx context.Context, id []byte) (registry.TxResult, error) {
	return l.submit(ctx, &settlementmsgs.MsgRefundSettlement{
		Opener: l.Owner(), SettlementId: id,
	}, "/dydxprotocol.settlement.MsgRefundSettlement")
}
