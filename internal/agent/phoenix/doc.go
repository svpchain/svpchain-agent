// Package phoenix optionally exports redacted assistant spans to Arize Phoenix
// over OTLP HTTP protobuf (application/x-protobuf). Empty Config.PhoenixOTLPURL
// disables it. Telemetry failures never abort a run. Private keys, API keys,
// signed_tx, and the system prompt body are never exported.
package phoenix
