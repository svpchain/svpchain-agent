// Package evmcall decodes the standard ERC-20 / ERC-721 / ERC-1155 transfer and
// approval call data shapes.
//
// It exists so the transfer whitelist can be enforced on the account that
// actually receives value or spending rights. In a token transaction the
// transaction's own `to` is the TOKEN CONTRACT, not the beneficiary: an ERC-20
// transfer(attacker, amount) names the attacker only inside the call data, and
// carries value 0. A whitelist check that looks only at `to` (and only when
// value > 0) therefore misses every token transfer and every approval — which is
// exactly the class of transaction the whitelist exists to constrain.
//
// Decoding is deliberately limited to the fixed set of standard selectors below.
// This is a whitelist-enforcement aid, not a general ABI decoder: an
// unrecognized selector yields no destination, and the caller decides what that
// means. Recognized-but-undecodable call data is an error, never "no
// destination" — see Decode.
//
// The package is pure (no I/O, no chain or config access) so both enforcement
// layers can use it: the signer in internal/signer and the assistant's
// pre-flight gate in internal/agent/guard.
package evmcall

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// Role names what a decoded address is being handed by the call: custody of
// tokens (recipient) or the right to move them later (spender / operator).
type Role string

const (
	RoleRecipient Role = "recipient"
	RoleSpender   Role = "spender"
	RoleOperator  Role = "operator"
)

// Destination is the account a guarded token call hands value or spending
// rights to. It is what the whitelist must be checked against.
type Destination struct {
	Address common.Address
	Role    Role
	// Method is the Solidity signature that was matched, for error messages.
	Method string
}

const (
	wordSize    = 32 // ABI head slot
	addressSize = 20
)

// methodSpec describes how to pull the guarded address out of one method's ABI
// head. Every argument — including dynamic ones like bytes or uint256[], which
// store an offset — occupies exactly one head word, so a fixed word index
// addresses the guarded argument regardless of the tail layout.
type methodSpec struct {
	name string
	// args is the total argument count, i.e. the number of head words the call
	// data must contain to be well-formed.
	args int
	// addrArg is the head-word index of the guarded address argument.
	addrArg int
	role    Role
	// revokeArg, when >= 0, is the head-word index of an amount/flag argument
	// whose zero value makes the call a REVOCATION (approve(x, 0),
	// setApprovalForAll(x, false)). Revocations are never gated: refusing them
	// would leave a user unable to withdraw an approval from a spender they no
	// longer trust — the whitelist must not be able to trap an allowance.
	revokeArg int
}

// guardedMethods maps a 4-byte selector to its spec. Selectors are the first 4
// bytes of keccak256 over the signature in the name field.
//
// Deliberately absent: decreaseAllowance(address,uint256) (0xa457c2d7), which
// can only reduce an existing allowance and so is a revocation in every case.
var guardedMethods = map[[4]byte]methodSpec{
	// ERC-20
	{0xa9, 0x05, 0x9c, 0xbb}: {name: "transfer(address,uint256)", args: 2, addrArg: 0, role: RoleRecipient, revokeArg: -1},
	{0x09, 0x5e, 0xa7, 0xb3}: {name: "approve(address,uint256)", args: 2, addrArg: 0, role: RoleSpender, revokeArg: 1},
	{0x39, 0x50, 0x93, 0x51}: {name: "increaseAllowance(address,uint256)", args: 2, addrArg: 0, role: RoleSpender, revokeArg: 1},

	// ERC-20 and ERC-721 share this selector; the guarded argument (the
	// destination) is at the same index in both.
	{0x23, 0xb8, 0x72, 0xdd}: {name: "transferFrom(address,address,uint256)", args: 3, addrArg: 1, role: RoleRecipient, revokeArg: -1},

	// ERC-721
	{0x42, 0x84, 0x2e, 0x0e}: {name: "safeTransferFrom(address,address,uint256)", args: 3, addrArg: 1, role: RoleRecipient, revokeArg: -1},
	{0xb8, 0x8d, 0x4f, 0xde}: {name: "safeTransferFrom(address,address,uint256,bytes)", args: 4, addrArg: 1, role: RoleRecipient, revokeArg: -1},

	// ERC-721 and ERC-1155 share setApprovalForAll.
	{0xa2, 0x2c, 0xb4, 0x65}: {name: "setApprovalForAll(address,bool)", args: 2, addrArg: 0, role: RoleOperator, revokeArg: 1},

	// ERC-1155
	{0xf2, 0x42, 0x43, 0x2a}: {name: "safeTransferFrom(address,address,uint256,uint256,bytes)", args: 5, addrArg: 1, role: RoleRecipient, revokeArg: -1},
	{0x2e, 0xb2, 0xc2, 0xd6}: {name: "safeBatchTransferFrom(address,address,uint256[],uint256[],bytes)", args: 5, addrArg: 1, role: RoleRecipient, revokeArg: -1},
}

// Decode extracts the whitelist-relevant destination from EVM call data.
//
// Three outcomes, and callers must distinguish all three:
//
//   - (nil, nil)  — not a guarded token call (unknown selector, empty data, or a
//     revocation). There is nothing for the whitelist to check here.
//   - (dest, nil) — a guarded call handing dest.Address value or spending rights.
//     The caller must check dest.Address against the whitelist.
//   - (nil, err)  — the selector IS a guarded method but its arguments cannot be
//     decoded. The caller must REFUSE: treating undecodable call data as
//     "nothing to check" would make truncated or non-canonical arguments a
//     bypass for the whole check.
func Decode(data []byte) (*Destination, error) {
	if len(data) < 4 {
		return nil, nil
	}
	var sel [4]byte
	copy(sel[:], data[:4])
	spec, ok := guardedMethods[sel]
	if !ok {
		return nil, nil
	}

	head := data[4:]
	if len(head) < wordSize*spec.args {
		return nil, fmt.Errorf(
			"call data for %s is truncated: %d argument bytes, want at least %d",
			spec.name, len(head), wordSize*spec.args)
	}

	if spec.revokeArg >= 0 && isZeroWord(word(head, spec.revokeArg)) {
		// approve(x, 0) / setApprovalForAll(x, false): withdraws rights rather
		// than granting them, so it is allowed to any address.
		return nil, nil
	}

	w := word(head, spec.addrArg)
	// An ABI address is left-padded with 12 zero bytes. Non-zero padding means
	// the argument is not a canonical address, so we cannot say which account
	// the call actually targets — refuse rather than truncate to a guess.
	if !isZeroWord(w[:wordSize-addressSize]) {
		return nil, fmt.Errorf(
			"%s argument of %s is not a canonical ABI address: padding is not zero",
			spec.role, spec.name)
	}

	return &Destination{
		Address: common.BytesToAddress(w),
		Role:    spec.role,
		Method:  spec.name,
	}, nil
}

// word returns head word i. Callers must have length-checked head.
func word(head []byte, i int) []byte {
	return head[wordSize*i : wordSize*(i+1)]
}

func isZeroWord(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
