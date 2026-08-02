package a2aserver

import "errors"

var (
	errChainIDRequired = errors.New("chain id is required (pass --chain-id or set agent_chain_id in prefs.json)")
	errEmptyMessage    = errors.New("message has no text content")
	errNoLLMKey        = errors.New("LLM API key is not configured — set llm_api_key in prefs.json (Settings tab)")
)
