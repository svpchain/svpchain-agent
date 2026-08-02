package delegation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"

	"github.com/svpchain/svpdt"

	"github.com/svpchain/svpchain-local-agent/internal/registry"
)

// DefaultTaskTTLSeconds bounds a per-task credential's life when the caller
// does not ask for a shorter one. Five minutes: long enough for a remote agent
// to execute, short enough that a leaked credential is nearly dead on arrival.
const DefaultTaskTTLSeconds = 300

// MintParams is one per-task credential grant, already user-approved.
type MintParams struct {
	// RootID anchors the on-chain delegation the credential descends from.
	RootID []byte
	// AudienceDID is the remote agent's DID — the only party that can use it.
	AudienceDID string
	// Actions/Skills/Subaccounts/Denoms/Contracts are the explicit grants.
	// Empty sets DENY in SVP-DT, so everything the task needs must be listed.
	Actions     []string
	Skills      []string
	Subaccounts []uint32
	Denoms      []string
	Contracts   []string
	// Budget caps what this one credential can commit, in chain coins.
	Budget []registry.Coin
	// SvcBudget caps buying downstream agent services. Usually empty.
	SvcBudget []registry.Coin
	// TTLSeconds bounds the credential's life; 0 means DefaultTaskTTLSeconds.
	// Clamped to the chain's MaxTokenTtlSeconds either way.
	TTLSeconds int64
	// Now is the mint time in unix seconds.
	Now int64
}

// Mint issues one single-use depth-1 credential under the user's delegation,
// signed with the user's own account key (the chain resolves the issuer DID
// through its x/auth fallback). Returns the base64 proof chain, root first —
// exactly the wire form remote agents expect in args.proof.
func (l *Lifecycle) Mint(ctx context.Context, p MintParams) ([]string, error) {
	if len(p.RootID) != 32 {
		return nil, fmt.Errorf("root id must be 32 bytes, got %d", len(p.RootID))
	}
	if strings.TrimSpace(p.AudienceDID) == "" {
		return nil, fmt.Errorf("audience DID is required")
	}
	if p.Now <= 0 {
		return nil, fmt.Errorf("mint time is required")
	}

	// The epoch is read fresh on every mint: a stale epoch mints a
	// valid-looking credential the chain rejects with DT_EPOCH_STALE.
	epoch, paused, err := l.Registry.Epoch(ctx, p.RootID)
	if err != nil {
		return nil, fmt.Errorf("read delegation epoch: %w", err)
	}
	if paused {
		return nil, fmt.Errorf("the delegation is paused; resume it before delegating tasks")
	}

	params, err := l.Registry.WalletParams(ctx)
	if err != nil {
		return nil, fmt.Errorf("read agentwallet params: %w", err)
	}
	ttl := p.TTLSeconds
	if ttl <= 0 {
		ttl = DefaultTaskTTLSeconds
	}
	if max := int64(params.MaxTokenTtlSeconds); max > 0 && ttl > max {
		ttl = max
	}

	caveats, err := buildCaveats(l.Owner(), p, ttl)
	if err != nil {
		return nil, err
	}

	var rootID [32]byte
	copy(rootID[:], p.RootID)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("draw nonce: %w", err)
	}

	dtSigner, err := svpdt.NewPrivateKeySigner(l.Priv.Bytes())
	if err != nil {
		return nil, fmt.Errorf("build credential signer: %w", err)
	}

	_, encoded, err := svpdt.Issue(dtSigner, svpdt.IssueParams{
		ChainID:   l.ChainID,
		Root:      rootID,
		RootEpoch: epoch,
		Issuer:    l.OwnerDID(),
		Audience:  strings.TrimSpace(p.AudienceDID),
		Caveats:   caveats,
		Nonce:     nonce,
	})
	if err != nil {
		return nil, fmt.Errorf("issue credential: %w", err)
	}

	return []string{base64.StdEncoding.EncodeToString(encoded)}, nil
}

// buildCaveats assembles the credential's grant. Everything is explicit —
// SVP-DT's empty sets deny, which is the property that makes a narrow
// per-task credential safe to hand out.
func buildCaveats(principal string, p MintParams, ttl int64) (svpdt.Caveats, error) {
	actions, err := svpdt.NewStringSet(p.Actions...)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("actions: %w", err)
	}
	skills, err := svpdt.NewStringSet(p.Skills...)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("skills: %w", err)
	}
	denoms, err := svpdt.NewStringSet(p.Denoms...)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("denoms: %w", err)
	}
	contracts, err := svpdt.NewStringSet(p.Contracts...)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("contracts: %w", err)
	}
	budget, err := toDTCoins(p.Budget)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("budget: %w", err)
	}
	svcBudget, err := toDTCoins(p.SvcBudget)
	if err != nil {
		return svpdt.Caveats{}, fmt.Errorf("svc budget: %w", err)
	}

	return svpdt.Caveats{
		Principal:   principal,
		Budget:      budget,
		SvcBudget:   svcBudget,
		Actions:     actions,
		Skills:      skills,
		Subaccounts: svpdt.NewUint32Set(p.Subaccounts...),
		Denoms:      denoms,
		Contracts:   contracts,
		// One hop, no re-delegation: the credential names its executor and
		// dies there. Multi-hop chains are a deliberate future decision, not
		// a default.
		Redelegable: false,
		MaxDepth:    1,
		NotBefore:   p.Now - 60,
		Expires:     p.Now + ttl,
	}, nil
}

func toDTCoins(coins []registry.Coin) (svpdt.Coins, error) {
	if len(coins) == 0 {
		return nil, nil
	}
	out := make([]svpdt.Coin, 0, len(coins))
	for _, c := range coins {
		amount, ok := new(big.Int).SetString(strings.TrimSpace(c.Amount), 10)
		if !ok {
			return nil, fmt.Errorf("amount %q is not a base-10 integer", c.Amount)
		}
		out = append(out, svpdt.Coin{Denom: strings.TrimSpace(c.Denom), Amount: amount})
	}
	return svpdt.NewCoins(out...)
}
