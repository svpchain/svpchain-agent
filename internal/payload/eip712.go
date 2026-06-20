package payload

// EIP712TypedData mirrors eth_signTypedData_v4 / viem signTypedData parameters.
// x402 "exact" scheme clients build this structure for EIP-3009 TransferWithAuthorization.
type EIP712TypedData struct {
	Types       map[string][]EIP712Type  `json:"types"`
	PrimaryType string                   `json:"primaryType"`
	Domain      EIP712Domain             `json:"domain"`
	Message     map[string]interface{}   `json:"message"`
}

// EIP712Type is one field in an EIP-712 type definition.
type EIP712Type struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EIP712Domain is the EIP-712 domain separator. ChainId is a decimal string or JSON number on the wire.
type EIP712Domain struct {
	Name              string      `json:"name,omitempty"`
	Version           string      `json:"version,omitempty"`
	ChainId           interface{} `json:"chainId,omitempty"`
	VerifyingContract string      `json:"verifyingContract,omitempty"`
	Salt              string      `json:"salt,omitempty"`
}

// SignedTypedData is returned after signing EIP-712 typed data.
type SignedTypedData struct {
	Signature string `json:"signature"` // 0x-prefixed 65-byte ECDSA signature (v = 27 or 28)
	Signer    string `json:"signer"`    // 0x-checksummed EVM address of the loaded key
}
