package a2acall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

func IsTool(name string) bool {
	return name == "a2a_send_message" || name == "a2a_build_lendora_collateral_tx"
}

func SendFromArgs(ctx context.Context, args map[string]any) (string, error) {
	agentURL, _ := args["agent_url"].(string)
	message, _ := args["message"].(string)
	if agentURL == "" {
		return "", fmt.Errorf("agent_url is required")
	}
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	return a2aSendMessage(ctx, agentURL, message)
}

// a2aSendMessage is overridden in tests.
var a2aSendMessage = func(ctx context.Context, agentURL, message string) (string, error) {
	return svpa2a.SendToAgentJSON(ctx, agentURL, message)
}

// BuildLendoraCollateral requests a collateral build from a registered lending
// agent and returns only a payload that is safe to admit to the local EVM
// write lane. The registry card hash anchors both the endpoint and declared
// tool surface; this is intentionally separate from raw a2a_send_message.
func BuildLendoraCollateral(ctx context.Context, args map[string]any, reg *registry.Client, localEVMOwner, localEVMChainID string) (payload.EvmTxPayload, map[string]string, error) {
	if reg == nil || reg.BaseURL() == "" {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("agent discovery is not configured: set the chain REST endpoint in Settings")
	}
	agentID, err := requiredString(args, "agent_id")
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}
	asset, err := requiredString(args, "asset")
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}
	action, err := requiredString(args, "action")
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}
	if action != "enable" && action != "disable" {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("action must be \"enable\" or \"disable\"")
	}
	clientID, err := requiredString(args, "client_id")
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}
	if !common.IsHexAddress(localEVMOwner) {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("local EVM signing identity is unavailable")
	}
	if strings.TrimSpace(localEVMChainID) == "" || localEVMChainID == "0" {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("local EVM signing chain is unavailable")
	}

	agent, err := reg.AgentByID(ctx, agentID)
	if err != nil {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("look up lending agent: %w", err)
	}
	if !agent.Active() {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("agent %s is not active", agentID)
	}
	card, err := reg.FetchCard(ctx, agent)
	if err != nil {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("fetch lending agent card: %w", err)
	}
	comptroller, err := lendingComptrollerFromCard(card)
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}

	req := map[string]any{
		"skill": "svpchain-lendora",
		"tool":  "lendora_build_collateral_tx",
		"args": map[string]string{
			"asset": asset, "action": action, "client_id": clientID,
		},
	}
	if bearer, _ := args["bearer"].(string); strings.TrimSpace(bearer) != "" {
		req["bearer"] = strings.TrimSpace(bearer)
	}
	message, err := json.Marshal(req)
	if err != nil {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("encode collateral request: %w", err)
	}
	result, err := sendToAgent(ctx, agent.Endpoint, string(message))
	if err != nil {
		return payload.EvmTxPayload{}, nil, fmt.Errorf("request collateral build: %w", err)
	}
	p, err := collateralPayloadFromResponse(result.Response, action, localEVMOwner, localEVMChainID, comptroller)
	if err != nil {
		return payload.EvmTxPayload{}, nil, err
	}
	return p, map[string]string{"agent_id": agent.AgentID, "agent_endpoint": agent.Endpoint}, nil
}

var sendToAgent = svpa2a.SendToAgent

type lendingCard struct {
	Skills []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	} `json:"skills"`
}

func lendingComptrollerFromCard(card registry.Card) (string, error) {
	if !card.Verified {
		return "", fmt.Errorf("registered lending agent card hash does not match its on-chain record")
	}
	var decoded lendingCard
	if err := json.Unmarshal(card.Raw, &decoded); err != nil {
		return "", fmt.Errorf("decode registered lending agent card: %w", err)
	}
	var hasCollateralBuilder bool
	for _, skill := range decoded.Skills {
		if skill.ID == "svpchain-lendora" && strings.Contains(skill.Description, "lendora_build_collateral_tx") {
			hasCollateralBuilder = true
		}
	}
	if !hasCollateralBuilder {
		return "", fmt.Errorf("registered agent card does not declare a Lendora collateral builder")
	}
	for _, skill := range decoded.Skills {
		if skill.ID != "svpchain-execution-lendora" {
			continue
		}
		const marker = "Runtime Lendora deployment: Comptroller "
		idx := strings.Index(skill.Description, marker)
		if idx < 0 {
			break
		}
		value := strings.Fields(strings.TrimPrefix(skill.Description[idx:], marker))
		if len(value) > 0 {
			address := strings.Trim(value[0], ";,.")
			if common.IsHexAddress(address) {
				return strings.ToLower(common.HexToAddress(address).Hex()), nil
			}
		}
		break
	}
	return "", fmt.Errorf("registered agent card does not declare its Lendora Comptroller")
}

func collateralPayloadFromResponse(response, action, localEVMOwner, localEVMChainID, comptroller string) (payload.EvmTxPayload, error) {
	var envelope struct {
		Skill  string          `json:"skill"`
		Tool   string          `json:"tool"`
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		return payload.EvmTxPayload{}, fmt.Errorf("decode lending agent response: %w", err)
	}
	if envelope.Skill != "svpchain-lendora" || envelope.Tool != "lendora_build_collateral_tx" {
		return payload.EvmTxPayload{}, fmt.Errorf("response is not from the requested Lendora collateral builder")
	}
	if !envelope.OK {
		return payload.EvmTxPayload{}, fmt.Errorf("lending agent refused collateral build: %s", envelope.Error)
	}
	var result struct {
		Payload *payload.EvmTxPayload `json:"payload"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return payload.EvmTxPayload{}, fmt.Errorf("decode Lendora collateral build: %w", err)
	}
	if result.Payload == nil {
		return payload.EvmTxPayload{}, fmt.Errorf("Lendora collateral build returned no signable payload")
	}
	p := *result.Payload
	if p.Summary.ToolName != "lendora_build_collateral_tx" {
		return payload.EvmTxPayload{}, fmt.Errorf("payload summary is not a Lendora collateral build")
	}
	if !common.IsHexAddress(p.SignerAddress) || common.HexToAddress(p.SignerAddress) != common.HexToAddress(localEVMOwner) {
		return payload.EvmTxPayload{}, fmt.Errorf("payload signer_address does not match the local EVM signer")
	}
	if p.EVMChainID != localEVMChainID {
		return payload.EvmTxPayload{}, fmt.Errorf("payload evm_chain_id does not match the local EVM signer")
	}
	if !common.IsHexAddress(p.To) || strings.ToLower(common.HexToAddress(p.To).Hex()) != comptroller {
		return payload.EvmTxPayload{}, fmt.Errorf("payload target is not the registered Lendora Comptroller")
	}
	if strings.TrimSpace(p.Value) != "" && strings.TrimSpace(p.Value) != "0" {
		return payload.EvmTxPayload{}, fmt.Errorf("Lendora collateral payload must not transfer native value")
	}
	data := common.FromHex(p.Data)
	if len(data) < 4 {
		return payload.EvmTxPayload{}, fmt.Errorf("Lendora collateral payload has no ABI selector")
	}
	wantMethod := "enterMarkets(address[])"
	if action == "disable" {
		wantMethod = "exitMarket(address)"
	}
	want := crypto.Keccak256([]byte(wantMethod))[:4]
	if string(data[:4]) != string(want) {
		return payload.EvmTxPayload{}, fmt.Errorf("payload selector is not %s", wantMethod)
	}
	return p, nil
}

func requiredString(args map[string]any, key string) (string, error) {
	v, _ := args[key].(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}
