package i18n

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/manage"
)

var (
	reDeleteKey        = regexp.MustCompile(`^delete key "(.+?)": (.+)$`)
	reReadKey          = regexp.MustCompile(`^read key "(.+?)" from OS credential store: (.+)$`)
	reUnknownAgent     = regexp.MustCompile(`^unknown ai agent: "(.+?)"$`)
	reAgentRounds      = regexp.MustCompile(`^agent exceeded (\d+) tool rounds$`)
	reThinking         = regexp.MustCompile(`^Thinking… \(round (\d+)\)$`)
	reCalling          = regexp.MustCompile(`^Calling (.+)$`)
	reToolFailed       = regexp.MustCompile(`^(.+?) failed$`)
	reToolOk           = regexp.MustCompile(`^(.+?) ok$`)
	reUnsupportedAddr  = regexp.MustCompile(`^unsupported address type "(.+?)"$`)
	reCosmosPrefix     = regexp.MustCompile(`^address must use the (.+?) bech32 prefix$`)
	reWhitelistCosmos  = regexp.MustCompile(`^recipient "(.+?)" is not on the whitelist for chain "(.+?)" \(SVP Cosmos\)$`)
	reWhitelistEVM     = regexp.MustCompile(`^recipient "(.+?)" is not on the whitelist for chain "(.+?)" \(EVM\)$`)
	reNoWhitelist      = regexp.MustCompile(`^no whitelist configured for chain "(.+?)" — add a recipient in the Security tab before transferring$`)
	reUpdateAsset      = regexp.MustCompile(`^release asset "(.+?)" not found$`)
	reUpdateChecksum   = regexp.MustCompile(`^SHA256SUMS has no entry for "(.+?)"$`)
	reUpdateNotSup     = regexp.MustCompile(`^in-app update not supported on (.+?)$`)
	reUpdateDownload   = regexp.MustCompile(`^download (.+?): (.+)$`)
	reUpdateHTTP       = regexp.MustCompile(`^download (.+?): HTTP (\d+): (.+)$`)
	reUpdateZip        = regexp.MustCompile(`^invalid zip entry: (.+)$`)
	reInternal         = regexp.MustCompile(`^internal error: (.+)$`)
	reTransferRejected = regexp.MustCompile(`^Transfer rejected — (.+?)\. No transaction was built, signed, or broadcast\.$`)
	reToolStopped      = regexp.MustCompile(`^(.+?) failed — (.+?)\. Stopped without further action\.$`)
)

// Localize maps a backend error to the active GUI language.
// Unrecognized errors are returned unchanged (usually English technical detail).
func Localize(err error) string {
	if err == nil {
		return ""
	}
	t := ErrT()

	if errors.Is(err, context.Canceled) {
		return t.ContextCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return t.ContextDeadline
	}

	var noKey *manage.NoSigningKeyError
	if errors.As(err, &noKey) {
		return FormatNoSigningKey(noKey.ChainID)
	}

	var rej *guard.Rejection
	if errors.As(err, &rej) && rej.Err != nil {
		return Localize(rej.Err)
	}

	switch {
	case errors.Is(err, ErrAgentBusy):
		return t.AgentBusy
	case errors.Is(err, ErrChainIDRequired):
		return t.ChainIDRequired
	case errors.Is(err, ErrMessageRequired):
		return t.MessageRequired
	case errors.Is(err, ErrLLMKeyRequired):
		return t.LLMAPIKeyNotConfigured
	}

	msg := err.Error()

	if s, ok := localizeExact(msg); ok {
		return s
	}
	if s, ok := localizePrefixes(msg); ok {
		return s
	}
	if s, ok := localizeRegex(msg); ok {
		return s
	}

	return msg
}

// LocalizeStepTitle translates known assistant step titles.
func LocalizeStepTitle(title string) string {
	if title == "" {
		return title
	}
	t := ErrT()
	switch title {
	case "Starting assistant…":
		return t.StartingAssistant
	case "LLM error":
		return t.LLMErrorTitle
	case "Stopped":
		return t.StoppedTitle
	case "Session context failed":
		return t.SessionContextFailed
	}
	if m := reThinking.FindStringSubmatch(title); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return fmt.Sprintf(t.ThinkingRoundFmt, n)
		}
	}
	if m := reCalling.FindStringSubmatch(title); len(m) == 2 {
		return fmt.Sprintf(t.CallingToolFmt, m[1])
	}
	if m := reToolFailed.FindStringSubmatch(title); len(m) == 2 {
		return fmt.Sprintf(t.ToolFailedTitleFmt, m[1])
	}
	if m := reToolOk.FindStringSubmatch(title); len(m) == 2 {
		return fmt.Sprintf(t.ToolOkFmt, m[1])
	}
	return title
}

// LocalizeAgentAnswer translates assistant stop/answer messages shown in the chat.
func LocalizeAgentAnswer(text string) string {
	if text == "" {
		return text
	}
	t := ErrT()
	if m := reTransferRejected.FindStringSubmatch(text); len(m) == 2 {
		return fmt.Sprintf(t.TransferRejectedFmt, LocalizeDetail(m[1]))
	}
	if m := reToolStopped.FindStringSubmatch(text); len(m) == 3 {
		return fmt.Sprintf(t.ToolStoppedDetailFmt, m[1], LocalizeDetail(m[2]))
	}
	return text
}

// LocalizeDetail localizes a single-line error fragment (e.g. step detail).
func LocalizeDetail(detail string) string {
	if detail == "" {
		return detail
	}
	return Localize(errors.New(detail))
}

func localizeExact(msg string) (string, bool) {
	t := ErrT()
	switch msg {
	case "chain id is required":
		return t.ChainIDRequired, true
	case "message is required":
		return t.MessageRequired, true
	case "LLM API key is not configured":
		return t.LLMAPIKeyNotConfigured, true
	case "signer binary path is required":
		return t.SignerPathRequired, true
	case "address is required":
		return t.AddressRequired, true
	case "invalid EVM address: must be a 0x-prefixed 20-byte hex string":
		return t.InvalidEVMAddress, true
	case "whitelist entry already exists":
		return t.WhitelistExists, true
	case "whitelist entry not found":
		return t.WhitelistNotFound, true
	case "remote mcp not connected":
		return t.RemoteMCPNotConn, true
	case "update info is nil":
		return t.UpdateInfoNil, true
	case "release missing tag_name":
		return t.UpdateMissingTag, true
	case "not running inside a macOS app bundle":
		return t.UpdateNotInBundle, true
	case "not running from a packaged Windows install folder":
		return t.UpdateNotInstalledLayout, true
	case "in-app update not supported on this platform":
		return t.UpdateUnsupportedPlatform, true
	}
	return "", false
}

func localizePrefixes(msg string) (string, bool) {
	t := ErrT()

	type rule struct {
		prefix string
		fmtKey func(rest string) string
	}
	rules := []rule{
		{"open OS credential store: ", func(r string) string { return fmt.Sprintf(t.OpenKeystoreFmt, r) }},
		{"invalid key: ", func(r string) string { return fmt.Sprintf(t.InvalidKeyFmt, r) }},
		{"store key: ", func(r string) string { return fmt.Sprintf(t.StoreKeyFmt, r) }},
		{"list keys: ", func(r string) string { return fmt.Sprintf(t.ListKeysFmt, r) }},
		{"parse key: ", func(r string) string { return fmt.Sprintf(t.ParseKeyFmt, r) }},
		{"remote mcp: ", func(r string) string { return fmt.Sprintf(t.RemoteMCPFmt, LocalizeDetail(r)) }},
		{"connect remote mcp: ", func(r string) string { return fmt.Sprintf(t.ConnectRemoteMCPFmt, r) }},
		{"load agent skills: ", func(r string) string { return fmt.Sprintf(t.LoadAgentSkillsFmt, r) }},
		{"signer_whoami: ", func(r string) string { return fmt.Sprintf(t.SignerWhoamiFmt, r) }},
		{"whoami: ", func(r string) string { return fmt.Sprintf(t.WhoamiFmt, r) }},
		{"invalid SVP Cosmos address: ", func(r string) string { return fmt.Sprintf(t.InvalidCosmosAddressFmt, r) }},
		{"fetch release: ", func(r string) string { return fmt.Sprintf(t.UpdateFetchReleaseFmt, r) }},
		{"decode release: ", func(r string) string { return fmt.Sprintf(t.UpdateDecodeReleaseFmt, r) }},
		{"hash release asset: ", func(r string) string { return fmt.Sprintf(t.UpdateChecksumMismatchFmt, r) }},
		{"checksum mismatch for ", func(r string) string { return fmt.Sprintf(t.UpdateChecksumMismatchFmt, strings.TrimSpace(r)) }},
		{"start update helper: ", func(r string) string { return fmt.Sprintf(t.UpdateInstallHelperFmt, r) }},
		{"hdiutil attach: ", func(r string) string { return fmt.Sprintf(t.UpdateHdiutilFmt, r) }},
		{"copy app from dmg: ", func(r string) string { return fmt.Sprintf(t.UpdateCopyFromDMGFmt, r) }},
	}
	for _, rule := range rules {
		if rest, ok := strings.CutPrefix(msg, rule.prefix); ok {
			return rule.fmtKey(rest), true
		}
	}

	if rest, ok := strings.CutPrefix(msg, "Session context failed"); ok {
		rest = strings.TrimPrefix(rest, ": ")
		if rest == "" {
			return t.SessionContextFailed, true
		}
		return fmt.Sprintf(t.SessionContextFmt, LocalizeDetail(rest)), true
	}

	if i := strings.Index(msg, " (reconnect: "); i > 0 {
		base := msg[:i]
		rest := msg[i+len(" (reconnect: "):]
		rest = strings.TrimSuffix(rest, ")")
		inner := Localize(errors.New(base))
		return fmt.Sprintf("%s (%s)", inner, fmt.Sprintf(t.RemoteMCPReconnect, rest)), true
	}

	return "", false
}

func localizeRegex(msg string) (string, bool) {
	t := ErrT()

	if m := reDeleteKey.FindStringSubmatch(msg); len(m) == 3 {
		return fmt.Sprintf(t.DeleteKeyFmt, m[1], m[2]), true
	}
	if m := reReadKey.FindStringSubmatch(msg); len(m) == 3 {
		return fmt.Sprintf(t.ReadKeyFmt, m[1], m[2]), true
	}
	if m := reUnknownAgent.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UnknownAgentFmt, m[1]), true
	}
	if m := reAgentRounds.FindStringSubmatch(msg); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return fmt.Sprintf(t.AgentMaxRoundsFmt, n), true
		}
	}
	if m := reUnsupportedAddr.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UnsupportedAddressTypeFmt, m[1]), true
	}
	if m := reCosmosPrefix.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.CosmosPrefixFmt, m[1]), true
	}
	if m := reWhitelistCosmos.FindStringSubmatch(msg); len(m) == 3 {
		return fmt.Sprintf(t.WhitelistCosmosFmt, m[1], m[2]), true
	}
	if m := reWhitelistEVM.FindStringSubmatch(msg); len(m) == 3 {
		return fmt.Sprintf(t.WhitelistEVMFmt, m[1], m[2]), true
	}
	if m := reNoWhitelist.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.NoWhitelistConfiguredFmt, m[1]), true
	}
	if m := reUpdateAsset.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UpdateAssetNotFoundFmt, m[1]), true
	}
	if m := reUpdateChecksum.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UpdateChecksumEntryFmt, m[1]), true
	}
	if m := reUpdateNotSup.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UpdateNotSupportedFmt, m[1]), true
	}
	if m := reUpdateDownload.FindStringSubmatch(msg); len(m) == 3 {
		return fmt.Sprintf(t.UpdateDownloadFmt, m[1], m[2]), true
	}
	if m := reUpdateHTTP.FindStringSubmatch(msg); len(m) == 4 {
		return fmt.Sprintf(t.UpdateHTTPFmt, m[2], m[3]), true
	}
	if m := reUpdateZip.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.UpdateInvalidZipFmt, m[1]), true
	}
	if m := reInternal.FindStringSubmatch(msg); len(m) == 2 {
		return fmt.Sprintf(t.InternalErrorFmt, m[1]), true
	}
	return "", false
}
