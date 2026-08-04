package desktop

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-local-agent/internal/delegation"
	"github.com/svpchain/svpchain-local-agent/internal/keystore"
	"github.com/svpchain/svpchain-local-agent/internal/manage"
	"github.com/svpchain/svpchain-local-agent/internal/registry"
	"github.com/svpchain/svpchain-local-agent/internal/signer"
)

// DelegationRow is one root delegation for the Delegations tab.
type DelegationRow struct {
	RootIDHex   string   `json:"root_id"`
	AgentID     string   `json:"agent_id"`
	SelfIssued  bool     `json:"self_issued"`
	Actions     []string `json:"actions"`
	Subaccounts []uint32 `json:"subaccounts"`
	TotalCap    string   `json:"total_cap"`
	DailyCap    string   `json:"daily_cap"`
	SpentTotal  string   `json:"spent_total"`
	Epoch       uint64   `json:"epoch"`
	Paused      bool     `json:"paused"`
	ExpiresAt   int64    `json:"expires_at"`
}

// lifecycle builds the delegation lifecycle for the configured chain, or
// explains what is missing.
func (a *App) lifecycle() (*delegation.Lifecycle, error) {
	settings := a.store.AgentSettings()
	restURL := strings.TrimSpace(settings.AgentHubURL)
	if restURL == "" {
		return nil, errors.New("chain REST endpoint is not configured; set it in Settings")
	}
	chainID := strings.TrimSpace(settings.ChainID)
	if chainID == "" {
		return nil, errors.New("chain id is not configured; set it in Settings")
	}
	var ring keyring.Keyring
	if r, err := keystore.Open(); err == nil {
		ring = r
	}
	hexKey, _, err := manage.SelectKey(ring, chainID, os.Getenv("SIGNER_KEY_HEX"))
	if err != nil {
		return nil, err
	}
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return nil, err
	}
	return &delegation.Lifecycle{
		Registry: registry.New(restURL),
		Priv:     priv,
		ChainID:  chainID,
	}, nil
}

// DelegationsList returns the user's root delegations.
func (a *App) DelegationsList() ([]DelegationRow, error) {
	l, err := a.lifecycle()
	if err != nil {
		return nil, localized(err)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	delegations, err := l.Registry.DelegationsByDelegator(ctx, l.Owner())
	if err != nil {
		return nil, err
	}
	rows := make([]DelegationRow, 0, len(delegations))
	for _, d := range delegations {
		row := DelegationRow{
			RootIDHex:   hex.EncodeToString(d.RootID),
			AgentID:     d.AgentID,
			SelfIssued:  d.AgentID == l.OwnerDID(),
			Actions:     d.Limits.Actions,
			Subaccounts: d.Limits.Subaccounts,
			TotalCap:    coinsListText(d.Limits.SpendLimitTotal),
			DailyCap:    coinsListText(d.Limits.SpendLimitDaily),
			Epoch:       uint64(d.Epoch),
			Paused:      d.Paused,
			ExpiresAt:   int64(d.ExpiresAt),
		}
		if spend, err := l.Registry.SpendByRoot(ctx, d.RootID); err == nil {
			row.SpentTotal = coinsListText(spend.Lifetime.Spent)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// DelegationPause is the emergency stop.
func (a *App) DelegationPause(rootIDHex string) (string, error) {
	return a.delegationOp(rootIDHex, (*delegation.Lifecycle).Pause)
}

// DelegationResume lifts a pause.
func (a *App) DelegationResume(rootIDHex string) (string, error) {
	return a.delegationOp(rootIDHex, (*delegation.Lifecycle).Resume)
}

// DelegationRevoke removes a delegation permanently.
func (a *App) DelegationRevoke(rootIDHex string) (string, error) {
	return a.delegationOp(rootIDHex, (*delegation.Lifecycle).Revoke)
}

func (a *App) delegationOp(
	rootIDHex string,
	op func(*delegation.Lifecycle, context.Context, []byte) (registry.TxResult, error),
) (string, error) {
	l, err := a.lifecycle()
	if err != nil {
		return "", localized(err)
	}
	rootID, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(rootIDHex), "0x"))
	if err != nil || len(rootID) != 32 {
		return "", errors.New("root id must be 32 bytes of hex")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	res, err := op(l, ctx, rootID)
	if err != nil {
		return "", err
	}
	return res.TxHash, nil
}

func coinsListText(coins []registry.Coin) string {
	if len(coins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(coins))
	for _, c := range coins {
		parts = append(parts, c.Amount+" "+c.Denom)
	}
	return strings.Join(parts, ", ")
}
