package evmcall_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-local-agent/internal/evmcall"
)

const (
	attacker = "0xAAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa"
	token    = "0xbBbBBBBbbBBBbbbBbbBbbbbbBBbBbbbbBbBbbBBb"
)

// pad left-pads b to a 32-byte ABI head word.
func pad(b []byte) []byte {
	w := make([]byte, 32)
	copy(w[32-len(b):], b)
	return w
}

func addrWord(a string) []byte { return pad(common.HexToAddress(a).Bytes()) }

func uintWord(n byte) []byte { return pad([]byte{n}) }

// selector derives the 4-byte selector from a Solidity signature, so these tests
// verify the hardcoded table against real keccak hashes rather than repeating
// the same constants the implementation uses.
func selector(sig string) []byte { return ethcrypto.Keccak256([]byte(sig))[:4] }

func call(sig string, words ...[]byte) []byte {
	out := selector(sig)
	for _, w := range words {
		out = append(out, w...)
	}
	return out
}

func TestDecode_GuardedMethods(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		role evmcall.Role
	}{
		{
			name: "erc20 transfer",
			data: call("transfer(address,uint256)", addrWord(attacker), uintWord(5)),
			role: evmcall.RoleRecipient,
		},
		{
			name: "erc20 approve",
			data: call("approve(address,uint256)", addrWord(attacker), uintWord(1)),
			role: evmcall.RoleSpender,
		},
		{
			name: "erc20 increaseAllowance",
			data: call("increaseAllowance(address,uint256)", addrWord(attacker), uintWord(1)),
			role: evmcall.RoleSpender,
		},
		{
			name: "transferFrom takes the second address",
			data: call("transferFrom(address,address,uint256)",
				addrWord(token), addrWord(attacker), uintWord(5)),
			role: evmcall.RoleRecipient,
		},
		{
			name: "erc721 safeTransferFrom",
			data: call("safeTransferFrom(address,address,uint256)",
				addrWord(token), addrWord(attacker), uintWord(1)),
			role: evmcall.RoleRecipient,
		},
		{
			name: "erc721 safeTransferFrom with data",
			data: call("safeTransferFrom(address,address,uint256,bytes)",
				addrWord(token), addrWord(attacker), uintWord(1), uintWord(0x80)),
			role: evmcall.RoleRecipient,
		},
		{
			name: "setApprovalForAll",
			data: call("setApprovalForAll(address,bool)", addrWord(attacker), uintWord(1)),
			role: evmcall.RoleOperator,
		},
		{
			name: "erc1155 safeTransferFrom",
			data: call("safeTransferFrom(address,address,uint256,uint256,bytes)",
				addrWord(token), addrWord(attacker), uintWord(1), uintWord(2), uintWord(0xa0)),
			role: evmcall.RoleRecipient,
		},
		{
			name: "erc1155 safeBatchTransferFrom",
			data: call("safeBatchTransferFrom(address,address,uint256[],uint256[],bytes)",
				addrWord(token), addrWord(attacker), uintWord(0xa0), uintWord(0xc0), uintWord(0xe0)),
			role: evmcall.RoleRecipient,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest, err := evmcall.Decode(tc.data)
			require.NoError(t, err)
			require.NotNil(t, dest, "guarded method must yield a destination")
			require.Equal(t, common.HexToAddress(attacker), dest.Address)
			require.Equal(t, tc.role, dest.Role)
			require.NotEmpty(t, dest.Method)
		})
	}
}

// The infinite-approval case from the reported issue: approve(attacker, 2^256-1).
func TestDecode_InfiniteApproval(t *testing.T) {
	maxUint := make([]byte, 32)
	for i := range maxUint {
		maxUint[i] = 0xff
	}
	dest, err := evmcall.Decode(call("approve(address,uint256)", addrWord(attacker), maxUint))
	require.NoError(t, err)
	require.NotNil(t, dest)
	require.Equal(t, evmcall.RoleSpender, dest.Role)
	require.Equal(t, common.HexToAddress(attacker), dest.Address)
}

func TestDecode_NotGuarded(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"nil data", nil},
		{"empty data", []byte{}},
		{"shorter than a selector", []byte{0x01, 0x02, 0x03}},
		{"unknown selector", call("mint(uint256)", uintWord(1))},
		{"balanceOf is read-only", call("balanceOf(address)", addrWord(attacker))},
		{
			// Reducing an allowance can only ever be safe, and must stay possible
			// for a spender the user has since removed from the whitelist.
			"decreaseAllowance",
			call("decreaseAllowance(address,uint256)", addrWord(attacker), uintWord(1)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest, err := evmcall.Decode(tc.data)
			require.NoError(t, err)
			require.Nil(t, dest)
		})
	}
}

// Revocations must never be gated: a user has to be able to withdraw an
// approval from a spender they no longer trust, whitelist or not.
func TestDecode_RevocationsAreNotGuarded(t *testing.T) {
	zero := pad(nil)

	dest, err := evmcall.Decode(call("approve(address,uint256)", addrWord(attacker), zero))
	require.NoError(t, err)
	require.Nil(t, dest, "approve(x, 0) revokes an allowance")

	dest, err = evmcall.Decode(call("setApprovalForAll(address,bool)", addrWord(attacker), zero))
	require.NoError(t, err)
	require.Nil(t, dest, "setApprovalForAll(x, false) revokes operator rights")

	// ...but granting still is.
	dest, err = evmcall.Decode(call("setApprovalForAll(address,bool)", addrWord(attacker), uintWord(1)))
	require.NoError(t, err)
	require.NotNil(t, dest)
}

// A known selector whose arguments will not decode must be an error, never a
// silent "nothing to check" — otherwise malformed call data bypasses the
// whitelist entirely.
func TestDecode_MalformedGuardedCallIsAnError(t *testing.T) {
	t.Run("truncated arguments", func(t *testing.T) {
		data := call("transfer(address,uint256)", addrWord(attacker)) // missing amount word
		dest, err := evmcall.Decode(data)
		require.Error(t, err)
		require.Nil(t, dest)
		require.Contains(t, err.Error(), "truncated")
	})

	t.Run("no arguments at all", func(t *testing.T) {
		dest, err := evmcall.Decode(selector("transfer(address,uint256)"))
		require.Error(t, err)
		require.Nil(t, dest)
	})

	t.Run("non-canonical address padding", func(t *testing.T) {
		dirty := addrWord(attacker)
		dirty[0] = 0x01 // junk in the 12-byte zero pad
		dest, err := evmcall.Decode(call("transfer(address,uint256)", dirty, uintWord(1)))
		require.Error(t, err)
		require.Nil(t, dest)
		require.Contains(t, err.Error(), "canonical")
	})
}

// Guards the hardcoded selector table against a typo, using well-known values.
func TestDecode_SelectorTableMatchesKeccak(t *testing.T) {
	known := map[string]string{
		"transfer(address,uint256)":             "0xa9059cbb",
		"approve(address,uint256)":              "0x095ea7b3",
		"transferFrom(address,address,uint256)": "0x23b872dd",
		"setApprovalForAll(address,bool)":       "0xa22cb465",
	}
	for sig, want := range known {
		require.Equal(t, want, hexutil.Encode(selector(sig)), sig)
	}
}
