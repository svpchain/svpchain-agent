# Lendora Delegated Execution

Use this reference only after `get_agent_card` confirms a registered agent
advertises the `svpchain-execution-lendora` skill and exposes the tool needed
for the requested operation. The card is the authority for Lendora contract
addresses, ABI method signatures, and argument shapes. Do not infer them from
an asset symbol or from a previous task.

## Contract-method credential

For an agent tool named `execute_evm_contract_method`, grant one Lendora
contract and one ABI selector. Both the root delegation and the per-task
credential must include:

```json
{
  "actions": ["evm.contract_call"],
  "subaccounts": [0],
  "contracts": ["<lowercase Lendora contract from the agent card>"]
}
```

The root delegation may omit spend limits when its only write action is
`evm.contract_call`. Do not add a generic budget to the task for a zero-value
contract call; the signed method grant, not a Cosmos-denom budget, authorizes
the call.

Create the task with the exact skill and tool advertised by the card:

```json
{
  "agent_id": "<registered agent DID>",
  "skill": "svpchain-execution-lendora",
  "tool": "execute_evm_contract_method",
  "args": {
    "call": {
      "contract": "<same lowercase Lendora contract>",
      "method": "<ABI signature from the agent card>",
      "args": ["<arguments in the card's required order>"]
    }
  },
  "actions": ["evm.contract_call"],
  "skills": ["svpchain-execution-lendora"],
  "subaccounts": [0],
  "contracts": ["<same lowercase Lendora contract>"]
}
```

`method` must be a whitespace-free ABI signature such as `mint(uint256)`;
the agent ABI-encodes `args`. The local app signs an SVP-DT Task caveat binding
the user's account, the contract, and that method's four-byte selector. The
agent cannot use that credential on a different contract or method, but the
selector does not constrain the method arguments. Review the amount, asset,
recipient, and any approval/spender values before approving the dialog.

## Raw calldata tool

Use `execute_evm_call` only when the card explicitly requires raw calldata.
Its `args.call` contains the same lowercase `contract` plus a `0x`-prefixed
`data` value with at least a four-byte selector. It receives the same
contract-and-selector Task binding. Do not construct calldata by hand when the
typed method tool is available.

## Lendora operation sequence

1. Inspect the agent card and select its advertised Lendora contract method.
2. Check the current root delegation. Create a new narrow root if it does not
   include the exact contract and `evm.contract_call` on subaccount `0`.
3. Call `delegate_task` with the exact card-provided contract, method, and
   arguments. The user must approve the delegation dialog.
4. Report the remote agent result verbatim, including any transaction hash.

For ERC-20 approvals, the contract in the grant is the token contract and the
method is the card-advertised approval method. For supply, borrow, repay,
withdraw, or collateral operations, use the contract and method supplied for
that operation by the agent card. A new contract or ABI selector requires a
new task credential and user approval.
