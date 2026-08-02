package i18n

// ErrorTexts holds user-visible error copy for the active language.
type ErrorTexts struct {
	NoSigningKey           string
	ChainIDRequired        string
	MessageRequired        string
	LLMAPIKeyNotConfigured string
	AgentBusy              string

	OpenKeystoreFmt string
	InvalidKeyFmt   string
	StoreKeyFmt     string
	ListKeysFmt     string
	DeleteKeyFmt    string
	ReadKeyFmt      string
	ParseKeyFmt     string

	SignerPathRequired string
	UnknownAgentFmt    string

	RemoteMCPFmt        string
	RemoteMCPNotConn    string
	RemoteMCPReconnect  string
	ConnectRemoteMCPFmt string
	LoadAgentSkillsFmt  string
	AgentMaxRoundsFmt   string
	SignerWhoamiFmt     string
	WhoamiFmt           string
	SessionContextFmt   string

	UnsupportedAddressTypeFmt string
	AddressRequired           string
	InvalidCosmosAddressFmt   string
	CosmosPrefixFmt           string
	InvalidEVMAddress         string
	WhitelistExists           string
	WhitelistNotFound         string
	WhitelistCosmosFmt        string
	WhitelistEVMFmt           string
	NoWhitelistConfiguredFmt  string

	UpdateInfoNil             string
	UpdateFetchReleaseFmt     string
	UpdateHTTPFmt             string
	UpdateDecodeReleaseFmt    string
	UpdateMissingTag          string
	UpdateAssetNotFoundFmt    string
	UpdateNotSupportedFmt     string
	UpdateUnsupportedPlatform string
	UpdateChecksumEntryFmt    string
	UpdateChecksumMismatchFmt string
	UpdateDownloadFmt         string
	UpdateNotInBundle         string
	UpdateNotInstalledLayout  string
	UpdateInstallHelperFmt    string
	UpdateInvalidZipFmt       string
	UpdateCopyFromDMGFmt      string
	UpdateHdiutilFmt          string

	InternalErrorFmt string
	ContextCancelled string
	ContextDeadline  string

	TransferRejectedFmt  string
	ToolFailedFmt        string
	ToolStoppedDetailFmt string

	LLMErrorTitle        string
	StartingAssistant    string
	ThinkingRoundFmt     string
	CallingToolFmt       string
	ToolFailedTitleFmt   string
	ToolOkFmt            string
	StoppedTitle         string
	SessionContextFailed string
}

var errorCatalog = map[Lang]ErrorTexts{
	Zh: {
		NoSigningKey:           "Chain ID %s 没有签名密钥：请打开 SVPChain Agent，进入左侧「密钥」页，选择 Chain ID %s，导入私钥或点击「自动生成」后保存到系统凭据库；无头环境可设置 SIGNER_KEY_HEX",
		ChainIDRequired:        "请选择 Chain ID — 可在「设置」或「助手」页顶部的下拉框中选择",
		MessageRequired:        "请输入消息",
		LLMAPIKeyNotConfigured: "尚未配置 LLM API Key — 请打开「设置」，在「大模型」区块填写 API Key 并点击「保存设置」",
		AgentBusy:              "助手正在运行中，请等待当前任务完成或先取消",

		OpenKeystoreFmt: "无法打开系统凭据库：%s",
		InvalidKeyFmt:   "私钥无效：%s",
		StoreKeyFmt:     "保存密钥失败：%s",
		ListKeysFmt:     "读取密钥列表失败：%s",
		DeleteKeyFmt:    "删除密钥 %s 失败：%s",
		ReadKeyFmt:      "从系统凭据库读取密钥 %s 失败：%s",
		ParseKeyFmt:     "解析私钥失败：%s",

		SignerPathRequired: "请选择签名程序路径",
		UnknownAgentFmt:    "未知的 AI Agent：%s",

		RemoteMCPFmt:        "远程 MCP 错误：%s",
		RemoteMCPNotConn:    "远程 MCP 未连接",
		RemoteMCPReconnect:  "远程 MCP 重连失败：%s",
		ConnectRemoteMCPFmt: "连接远程 MCP 失败：%s",
		LoadAgentSkillsFmt:  "加载助手 Skills 失败：%s",
		AgentMaxRoundsFmt:   "助手已超过 %d 轮工具调用上限",
		SignerWhoamiFmt:     "获取本地签名者信息失败：%s",
		WhoamiFmt:           "获取远端账户信息失败：%s",
		SessionContextFmt:   "会话上下文失败：%s",

		UnsupportedAddressTypeFmt: "不支持的地址类型：%s",
		AddressRequired:           "请输入地址",
		InvalidCosmosAddressFmt:   "SVP Cosmos 地址无效：%s",
		CosmosPrefixFmt:           "地址须使用 %s bech32 前缀",
		InvalidEVMAddress:         "EVM 地址无效：须为 0x 开头的 20 字节十六进制",
		WhitelistExists:           "该白名单条目已存在",
		WhitelistNotFound:         "未找到该白名单条目",
		WhitelistCosmosFmt:        "收款地址 %s 不在链 %s 的白名单中（SVP Cosmos）",
		WhitelistEVMFmt:           "收款地址 %s 不在链 %s 的白名单中（EVM）",
		NoWhitelistConfiguredFmt:  "链 %s 尚未配置转账白名单 — 请先在「安全」页添加收款地址",

		UpdateInfoNil:             "更新信息无效",
		UpdateFetchReleaseFmt:     "获取版本发布信息失败：%s",
		UpdateHTTPFmt:             "下载失败（HTTP %s）：%s",
		UpdateDecodeReleaseFmt:    "解析版本发布信息失败：%s",
		UpdateMissingTag:          "发布信息缺少版本号",
		UpdateAssetNotFoundFmt:    "未找到更新包：%s",
		UpdateNotSupportedFmt:     "当前系统（%s）不支持应用内更新",
		UpdateUnsupportedPlatform: "当前平台不支持应用内更新",
		UpdateChecksumEntryFmt:    "校验文件中缺少 %s 的条目",
		UpdateChecksumMismatchFmt: "安装包校验失败：%s",
		UpdateDownloadFmt:         "下载 %s 失败：%s",
		UpdateNotInBundle:         "当前未从 macOS 应用包内运行，无法自动更新",
		UpdateNotInstalledLayout:  "当前未从已安装的发布目录运行，无法自动更新",
		UpdateInstallHelperFmt:    "启动更新程序失败：%s",
		UpdateInvalidZipFmt:       "无效的安装包条目：%s",
		UpdateCopyFromDMGFmt:      "从磁盘映像复制应用失败：%s",
		UpdateHdiutilFmt:          "挂载磁盘映像失败：%s",

		InternalErrorFmt: "内部错误：%s",
		ContextCancelled: "已取消",
		ContextDeadline:  "操作超时",

		TransferRejectedFmt:  "转账被拒绝 — %s。未构建、签名或广播任何交易。",
		ToolFailedFmt:        "%s 执行失败 — %s",
		ToolStoppedDetailFmt: "%s 失败 — %s。已停止后续操作。",

		LLMErrorTitle:        "大模型错误",
		StartingAssistant:    "正在启动助手…",
		ThinkingRoundFmt:     "思考中…（第 %d 轮）",
		CallingToolFmt:       "正在调用 %s",
		ToolFailedTitleFmt:   "%s 失败",
		ToolOkFmt:            "%s 完成",
		StoppedTitle:         "已停止",
		SessionContextFailed: "会话上下文失败",
	},
	En: {
		NoSigningKey:           "No signing key for %s: open SVPChain Agent, go to the Keys tab, select Chain ID %s, import a private key or use Auto-generate to save it to the OS credential store; for headless use, set SIGNER_KEY_HEX",
		ChainIDRequired:        "Chain id is required — select one in Settings or the Assistant tab",
		MessageRequired:        "Message is required",
		LLMAPIKeyNotConfigured: "LLM API key is not configured — open Settings, enter your key in the LLM section, and click Save Settings",
		AgentBusy:              "Assistant is already running",

		OpenKeystoreFmt: "Could not open OS credential store: %s",
		InvalidKeyFmt:   "Invalid private key: %s",
		StoreKeyFmt:     "Could not store key: %s",
		ListKeysFmt:     "Could not list keys: %s",
		DeleteKeyFmt:    "Could not delete key %s: %s",
		ReadKeyFmt:      "Could not read key %s from OS credential store: %s",
		ParseKeyFmt:     "Could not parse private key: %s",

		SignerPathRequired: "Signer binary path is required",
		UnknownAgentFmt:    "Unknown AI agent: %s",

		RemoteMCPFmt:        "Remote MCP error: %s",
		RemoteMCPNotConn:    "Remote MCP not connected",
		RemoteMCPReconnect:  "Remote MCP reconnect failed: %s",
		ConnectRemoteMCPFmt: "Could not connect to remote MCP: %s",
		LoadAgentSkillsFmt:  "Could not load agent skills: %s",
		AgentMaxRoundsFmt:   "Assistant exceeded %d tool-call rounds",
		SignerWhoamiFmt:     "signer_whoami failed: %s",
		WhoamiFmt:           "whoami failed: %s",
		SessionContextFmt:   "Session context failed: %s",

		UnsupportedAddressTypeFmt: "Unsupported address type %q",
		AddressRequired:           "Address is required",
		InvalidCosmosAddressFmt:   "Invalid SVP Cosmos address: %s",
		CosmosPrefixFmt:           "Address must use the %s bech32 prefix",
		InvalidEVMAddress:         "Invalid EVM address: must be a 0x-prefixed 20-byte hex string",
		WhitelistExists:           "Whitelist entry already exists",
		WhitelistNotFound:         "Whitelist entry not found",
		WhitelistCosmosFmt:        "Recipient %q is not on the whitelist for chain %q (SVP Cosmos)",
		WhitelistEVMFmt:           "Recipient %q is not on the whitelist for chain %q (EVM)",
		NoWhitelistConfiguredFmt:  "No whitelist configured for chain %q — add a recipient in the Security tab before transferring",

		UpdateInfoNil:             "Update info is nil",
		UpdateFetchReleaseFmt:     "Could not fetch release: %s",
		UpdateHTTPFmt:             "Download failed (HTTP %s): %s",
		UpdateDecodeReleaseFmt:    "Could not decode release: %s",
		UpdateMissingTag:          "Release missing tag_name",
		UpdateAssetNotFoundFmt:    "Release asset %q not found",
		UpdateNotSupportedFmt:     "In-app update not supported on %s",
		UpdateUnsupportedPlatform: "In-app update not supported on this platform",
		UpdateChecksumEntryFmt:    "SHA256SUMS has no entry for %q",
		UpdateChecksumMismatchFmt: "Checksum mismatch for %s",
		UpdateDownloadFmt:         "Could not download %s: %s",
		UpdateNotInBundle:         "Not running inside a macOS app bundle",
		UpdateNotInstalledLayout:  "Not running from a packaged Windows install folder",
		UpdateInstallHelperFmt:    "Could not start update helper: %s",
		UpdateInvalidZipFmt:       "Invalid zip entry: %s",
		UpdateCopyFromDMGFmt:      "Could not copy app from disk image: %s",
		UpdateHdiutilFmt:          "Could not attach disk image: %s",

		InternalErrorFmt: "Internal error: %s",
		ContextCancelled: "Cancelled",
		ContextDeadline:  "Timed out",

		TransferRejectedFmt:  "Transfer rejected — %s. No transaction was built, signed, or broadcast.",
		ToolFailedFmt:        "%s failed — %s",
		ToolStoppedDetailFmt: "%s failed — %s. Stopped without further action.",

		LLMErrorTitle:        "LLM error",
		StartingAssistant:    "Starting assistant…",
		ThinkingRoundFmt:     "Thinking… (round %d)",
		CallingToolFmt:       "Calling %s",
		ToolFailedTitleFmt:   "%s failed",
		ToolOkFmt:            "%s ok",
		StoppedTitle:         "Stopped",
		SessionContextFailed: "Session context failed",
	},
}

// ErrT returns localized error templates for the active language.
func ErrT() ErrorTexts { return errorCatalog[active] }
