package i18n

import "errors"

// GUI validation sentinels — Error() stays English for logs; Localize() maps them.
var (
	ErrAgentBusy       = errors.New("assistant is already running")
	ErrChainIDRequired = errors.New("chain id is required")
	ErrMessageRequired = errors.New("message is required")
	ErrLLMKeyRequired  = errors.New("llm api key is not configured")
)
