// Package history persists assistant conversations and manages the LLM context
// window across turns.
//
// Layout (next to prefs.json):
//
//	sessions/
//	  index.json          — session list + current session id
//	  <id>.jsonl          — one Entry per line (user/assistant/tool messages)
//	  <id>.summary.json   — compaction state (summary text + covered entry count)
//	  blobs/<name>.txt    — full tool results too large for the context (projection)
//
// Context assembly per turn: optional summary block (compacted old turns) +
// entries recorded after the summary cut. Large tool results are truncated in
// the context and archived as blobs ("read projection"); the live run always
// saw the full result, only later turns see the truncated view.
//
// Compaction: when the estimated context exceeds the token budget, all but the
// most recent turns are summarized by the LLM into a single summary block that
// replaces them. Addresses, tx hashes, amounts, and explicit user constraints
// are required verbatim in the summary prompt.
//
// Privacy: user messages are key-redacted before persisting (64-hex sequences),
// matching the signer's "keys never touch disk" rule. Tool results and
// assistant answers are stored as-is (tx hashes must survive).
package history
