package i18n

import (
	"os"
	"strings"

	"github.com/jeandeaual/go-locale"
)

// Lang is a supported language code.
type Lang string

const (
	Zh Lang = "zh"
	En Lang = "en"
)

// Strings holds all user-visible GUI copy.
type Strings struct {
	WindowTitle string

	TabKeys     string
	TabImport   string
	TabConfig   string
	TabSettings string
	TabAbout    string

	AboutMarkdown string

	ColChainID    string
	ColCosmosAddr string
	ColEVMAddr    string
	BtnRefresh    string
	BtnDelete     string

	ChainIDLabel       string
	PrivateKeyLabel    string
	ChainIDPlaceholder string
	KeyPlaceholder     string
	BtnImport          string
	ImportHint         string

	ChainConfigPlaceholder string
	BinaryPlaceholder      string
	BtnBrowseBinary        string
	SignerPathLabel        string
	AgentLabel             string
	BtnGenerateConfig      string
	BtnCopyClipboard       string
	ConfigHint             string

	StatusReadKeysFailed  string
	StatusNoKeys          string
	StatusKeyCount        string
	StatusSelectToDelete  string
	StatusDeleted         string
	StatusEnterChainID    string
	StatusEnterKey        string
	StatusGenerateFirst   string
	StatusSelectChainID   string
	StatusSelectAgent     string
	StatusCopied          string
	StatusAddressCopied   string
	StatusSavedKey        string
	StatusConfigGenerated string
	StatusConflictSuffix  string

	DialogConfirmDeleteTitle string
	DialogConfirmDeleteBody  string
	DialogConflictTitle      string

	LangLabel string

	UpdateAvailableTitle   string
	UpdateAvailableBody    string
	UpdateUpgrade          string
	UpdateLater            string
	UpdateSkipVersion      string
	UpdateDownloadingTitle string
	UpdateDownloadingBody  string
	UpdateVerifyingBody    string
	UpdateExtractingBody   string
	UpdateReadyTitle       string
	UpdateReadyBody        string
	UpdateInstall          string
	UpdateCancel           string
	UpdateFailedTitle      string
	UpdateFailedBody       string
	UpdateOpenRelease      string
}

var active Lang
var catalog = map[Lang]Strings{
	Zh: {
		WindowTitle: "svpchain agent",

		TabKeys:     "已存密钥",
		TabImport:   "导入密钥",
		TabConfig:   "MCP 配置",
		TabSettings: "设置",
		TabAbout:    "说明",

		AboutMarkdown: `**svpchain agent**

svpchain 的本地密钥链上助手（Cosmos/EVM）。私钥保存在系统凭据库（macOS 钥匙串等），永不写入配置文件或传给远程服务。

**核心能力**
- **AI 助手**：用自然语言驱动链上操作。远程 MCP 负责构建交易与市场数据，本地签名；流程为 build → sign → broadcast。
- **密钥管理**：按 Chain ID 导入、查看 Cosmos（svp1…）与 EVM 地址，支持轮换与删除。
- **MCP 配置**：生成 Cursor 等客户端的 stdio 配置，供外部 AI Agent 调用本地签名服务。

**信任边界**：签名仅在本地完成（stdio，无网络端口）；远程服务通过 challenge 鉴权，不持有你的私钥。

**快速开始**：导入密钥 → 在「设置」配置 LLM API Key → 在「助手」发起链上操作；也可将 MCP 配置粘贴到 Cursor 等客户端。`,

		LangLabel: "语言",

		ColChainID:    "Chain ID",
		ColCosmosAddr: "SVP Cosmos 地址",
		ColEVMAddr:    "EVM 地址",
		BtnRefresh:    "刷新",
		BtnDelete:     "删除选中",

		ChainIDLabel:       "Chain ID",
		PrivateKeyLabel:    "私钥",
		ChainIDPlaceholder: "可从列表选择或自行输入",
		KeyPlaceholder:     "32 字节私钥（hex，可带 0x 前缀）",
		BtnImport:          "导入到系统凭据库",
		ImportHint:         "同一 Chain ID 再次导入会覆盖旧密钥（轮换）。",

		ChainConfigPlaceholder: "与已导入密钥对应的 chain id",
		BinaryPlaceholder:      "svpchain-mcp 可执行文件的绝对路径",
		BtnBrowseBinary:        "选择签名程序…",
		SignerPathLabel:        "签名程序路径",
		AgentLabel:             "AI Agent",
		BtnGenerateConfig:      "生成配置",
		BtnCopyClipboard:       "复制到剪贴板",
		ConfigHint:             "将生成的 JSON 合并到 Cursor 的 MCP 设置中即可。",

		StatusReadKeysFailed:  "读取密钥失败: %v",
		StatusNoKeys:          "尚未导入任何密钥",
		StatusKeyCount:        "共 %d 个已存密钥",
		StatusSelectToDelete:  "请先在列表中选择要删除的 Chain ID",
		StatusDeleted:         "已删除 %q",
		StatusEnterChainID:    "请输入 Chain ID",
		StatusEnterKey:        "请输入私钥",
		StatusGenerateFirst:   "请先生成配置",
		StatusSelectChainID:   "请选择 Chain ID",
		StatusSelectAgent:     "请选择 AI Agent",
		StatusCopied:          "已复制 MCP 配置到剪贴板",
		StatusAddressCopied:   "已复制地址到剪贴板",
		StatusSavedKey:        "已保存密钥，Cosmos %s，EVM %s（%s）",
		StatusConfigGenerated: "配置已生成，可点击「复制到剪贴板」",
		StatusConflictSuffix:  "；警告：该密钥已用于 %s，跨链复用不推荐",

		DialogConfirmDeleteTitle: "确认删除",
		DialogConfirmDeleteBody:  "确定删除 Chain ID %q 的密钥？此操作不可恢复。",
		DialogConflictTitle:      "跨链复用警告",

		UpdateAvailableTitle:   "发现新版本",
		UpdateAvailableBody:    "当前版本 %s，最新版本 %s。是否现在更新？",
		UpdateUpgrade:          "升级",
		UpdateLater:            "稍后",
		UpdateSkipVersion:      "跳过 %s",
		UpdateDownloadingTitle: "正在下载更新",
		UpdateDownloadingBody:  "正在下载安装包…",
		UpdateVerifyingBody:    "正在校验安装包…",
		UpdateExtractingBody:   "正在解压安装包…",
		UpdateReadyTitle:       "准备安装",
		UpdateReadyBody:        "更新已下载并校验。应用将关闭并自动重启以完成安装。",
		UpdateInstall:          "立即安装",
		UpdateCancel:           "取消",
		UpdateFailedTitle:      "更新失败",
		UpdateFailedBody:       "无法完成更新：%v\n\n可在浏览器中打开 Release 页面手动下载。",
		UpdateOpenRelease:      "打开 Release 页面",
	},
	En: {
		WindowTitle: "svpchain agent",

		TabKeys:     "Stored Keys",
		TabImport:   "Import Key",
		TabConfig:   "MCP Config",
		TabSettings: "Settings",
		TabAbout:    "About",

		AboutMarkdown: `**svpchain agent**

A local-key on-chain assistant for svpchain (Cosmos/EVM). Private keys live in the OS credential store (macOS Keychain, etc.) — never in config files and never sent to remote services.

**What it does**
- **AI assistant** — Drive on-chain actions in natural language. A remote MCP builds transactions and serves market data; signing stays local. Flow: build → sign → broadcast.
- **Key management** — Import, view, rotate, and delete keys per Chain ID; see derived Cosmos (svp1…) and EVM addresses.
- **MCP export** — Generate stdio JSON for Cursor and other MCP clients to call the local signer.

**Trust model** — Signing runs only on your machine (stdio, no network port). Remote services authenticate via signed challenges and never hold your key.

**Get started** — Import a key → set your LLM API key in Settings → use the Assistant tab for on-chain actions; or paste the MCP config into Cursor.`,

		LangLabel: "Language",

		ColChainID:    "Chain ID",
		ColCosmosAddr: "SVP Cosmos Address",
		ColEVMAddr:    "EVM Address",
		BtnRefresh:    "Refresh",
		BtnDelete:     "Delete Selected",

		ChainIDLabel:       "Chain ID",
		PrivateKeyLabel:    "Private Key",
		ChainIDPlaceholder: "Pick from list or type your own",
		KeyPlaceholder:     "32-byte private key (hex, optional 0x prefix)",
		BtnImport:          "Import to OS Credential Store",
		ImportHint:         "Importing again under the same Chain ID overwrites the key (rotation).",

		ChainConfigPlaceholder: "Chain id matching an imported key",
		BinaryPlaceholder:      "Absolute path to svpchain-mcp",
		BtnBrowseBinary:        "Choose signer binary…",
		SignerPathLabel:        "Signer binary path",
		AgentLabel:             "AI Agent",
		BtnGenerateConfig:      "Generate Config",
		BtnCopyClipboard:       "Copy to Clipboard",
		ConfigHint:             "Merge the generated JSON into your Cursor MCP settings.",

		StatusReadKeysFailed:  "Failed to read keys: %v",
		StatusNoKeys:          "No keys imported yet",
		StatusKeyCount:        "%d key(s) stored",
		StatusSelectToDelete:  "Select a Chain ID in the list to delete",
		StatusDeleted:         "Deleted %q",
		StatusEnterChainID:    "Enter Chain ID",
		StatusEnterKey:        "Enter private key",
		StatusGenerateFirst:   "Generate config first",
		StatusSelectChainID:   "Select a Chain ID",
		StatusSelectAgent:     "Select an AI agent",
		StatusCopied:          "MCP config copied to clipboard",
		StatusAddressCopied:   "Address copied to clipboard",
		StatusSavedKey:        "Key saved, Cosmos %s, EVM %s (%s)",
		StatusConfigGenerated: "Config generated — click Copy to Clipboard",
		StatusConflictSuffix:  "; warning: key already used for %s — cross-chain reuse is discouraged",

		DialogConfirmDeleteTitle: "Confirm Delete",
		DialogConfirmDeleteBody:  "Delete the key for Chain ID %q? This cannot be undone.",
		DialogConflictTitle:      "Cross-Chain Reuse Warning",

		UpdateAvailableTitle:   "Update Available",
		UpdateAvailableBody:    "You are on %s. Version %s is available. Update now?",
		UpdateUpgrade:          "Upgrade",
		UpdateLater:            "Later",
		UpdateSkipVersion:      "Skip %s",
		UpdateDownloadingTitle: "Downloading Update",
		UpdateDownloadingBody:  "Downloading package…",
		UpdateVerifyingBody:    "Verifying package…",
		UpdateExtractingBody:   "Extracting package…",
		UpdateReadyTitle:       "Ready to Install",
		UpdateReadyBody:        "The update was downloaded and verified. The app will quit and restart to finish installing.",
		UpdateInstall:          "Install Now",
		UpdateCancel:           "Cancel",
		UpdateFailedTitle:      "Update Failed",
		UpdateFailedBody:       "Could not complete the update: %v\n\nYou can open the Release page in your browser to download manually.",
		UpdateOpenRelease:      "Open Release Page",
	},
}

// PrefKeyLanguage is the Fyne preferences key for the saved language.
const PrefKeyLanguage = "language"

// Init detects language and sets the active string catalog. Prefers SVPCHAIN_AGENT_LANG, then system locale.
func Init() Lang {
	active = detectLang()
	return active
}

// InitWithPreference prefers a saved language preference; otherwise behaves like Init.
func InitWithPreference(saved string) Lang {
	if lang, ok := ParseLang(saved); ok {
		active = lang
		return active
	}
	return Init()
}

// SetLang sets the active language.
func SetLang(lang Lang) {
	active = lang
}

// ParseLang parses a language code (zh, en, etc.).
func ParseLang(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "zh", "zh-hans", "zh-cn", "chinese":
		return Zh, true
	case "en", "english":
		return En, true
	default:
		if isChineseTag(s) {
			return Zh, true
		}
		if strings.HasPrefix(s, "en") {
			return En, true
		}
		return "", false
	}
}

// Current returns the active language.
func Current() Lang { return active }

// T returns the string catalog for the active language.
func T() Strings { return catalog[active] }

func detectLang() Lang {
	if v := strings.TrimSpace(os.Getenv("SVPCHAIN_AGENT_LANG")); v != "" {
		if isChineseTag(v) {
			return Zh
		}
		return En
	}
	if loc, err := locale.GetLocale(); err == nil && isChineseTag(loc) {
		return Zh
	}
	for _, key := range []string{"LC_ALL", "LANG", "LC_MESSAGES", "LANGUAGE"} {
		if isChineseTag(os.Getenv(key)) {
			return Zh
		}
	}
	return En
}

func isChineseTag(tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	return strings.HasPrefix(tag, "zh")
}
