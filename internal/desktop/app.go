// Package desktop implements the Wails-bound application layer for the graphical
// setup tool. It exposes key management (internal/manage) and in-app update
// (internal/update) to the Vue frontend without changing any signing logic.
package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/svpchain/svpchain-agent/internal/agent"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/brand"
	"github.com/svpchain/svpchain-agent/internal/i18n"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/update"
)

// App is the Wails application context; its exported methods are bound to JS.
type App struct {
	ctx   context.Context
	store *prefs.Store
}

// NewApp constructs the application with persisted preferences loaded.
func NewApp() *App {
	store := prefs.Load()
	skills.ApplySkillsConfigBase(store.File().SkillsConfigBase)
	return &App{store: store}
}

// Startup is wired to Wails OnStartup; it captures the runtime context.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Shutdown releases pooled remote MCP sessions when the app exits.
func (a *App) Shutdown(ctx context.Context) {
	agent.ShutdownRemotePool()
}

// --- metadata ---

// CurrentVersion returns the build version.
func (a *App) CurrentVersion() string { return Version }

// WindowTitle returns the application window title.
func (a *App) WindowTitle() string { return brand.AppDisplayName }

// --- i18n ---

// Language returns the effective language ("zh"/"en"), preferring the saved preference,
// then the system locale.
func (a *App) Language() string {
	return string(i18n.InitWithPreference(a.store.File().Language))
}

// SetLanguage persists the chosen language.
func (a *App) SetLanguage(lang string) {
	if l, ok := i18n.ParseLang(lang); ok {
		i18n.SetLang(l)
		a.store.SetLanguage(string(l))
	}
}

// --- key management (internal/manage, unchanged logic) ---

// DefaultChainIDs returns the default Chain ID suggestions for import.
func (a *App) DefaultChainIDs() []string { return manage.DefaultImportChainIDs }

// AgentNames returns the supported AI agent names.
func (a *App) AgentNames() []string { return manage.AgentNames }

// DefaultAgent returns the default selected AI agent.
func (a *App) DefaultAgent() string { return manage.AgentNameCursor }

// ListKeys returns all stored keys with derived addresses.
func (a *App) ListKeys() ([]manage.Entry, error) { return manage.List() }

// ImportKey stores hexKey under chainID and returns derived addresses + reuse warnings.
func (a *App) ImportKey(chainID, hexKey string) (manage.ImportResult, error) {
	return manage.Import(chainID, hexKey)
}

// GenerateKey returns a fresh random private key as 0x-prefixed hex for the import field.
func (a *App) GenerateKey() (string, error) { return manage.GenerateKey() }

// DeleteKey removes the key stored under chainID.
func (a *App) DeleteKey(chainID string) error { return manage.Delete(chainID) }

// GenerateConfig returns the MCP client config text for the agent/chain/binary.
func (a *App) GenerateConfig(agent, chainID, signerPath string) (string, error) {
	return manage.MCPConfigText(agent, chainID, signerPath)
}

// GuessSignerPath best-effort locates the signer binary next to this executable.
func (a *App) GuessSignerPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return manage.GuessSignerBinaryPath(exe)
}

// BrowseSignerBinary opens a native file picker and returns the chosen path.
func (a *App) BrowseSignerBinary() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose signer binary",
	})
}

// --- clipboard / browser ---

// CopyText writes text to the system clipboard.
func (a *App) CopyText(text string) error {
	return wruntime.ClipboardSetText(a.ctx, text)
}

// OpenURL opens url in the default browser.
func (a *App) OpenURL(url string) {
	if url != "" {
		wruntime.BrowserOpenURL(a.ctx, url)
	}
}

// --- in-app update (internal/update, unchanged logic) ---

// UpdateEnabled reports whether in-app updates are supported on this build/runtime.
func (a *App) UpdateEnabled() bool { return update.Enabled() }

// CheckUpdate returns update info if a newer release is available, else null.
func (a *App) CheckUpdate() (*update.Info, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return update.Check(ctx, Version, a.store.File().SkipVersion, nil)
}

// SkipVersion suppresses future prompts for the given release tag.
func (a *App) SkipVersion(tag string) { a.store.SetSkipVersion(tag) }

// StartUpdate downloads, verifies, and extracts the release, emitting
// "update:progress" events. It returns the staged .app path on success.
func (a *App) StartUpdate(info *update.Info) (string, error) {
	if info == nil {
		return "", fmt.Errorf("update info is nil")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Minute)
	defer cancel()

	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	staging := filepath.Join(cache, "com.svpchain.agent-gui", "update", info.Latest)

	progress := func(done, total int64) {
		if total <= 0 {
			return
		}
		stage := "downloading"
		switch {
		case done >= 850:
			stage = "extracting"
		case done >= 760:
			stage = "verifying"
		}
		wruntime.EventsEmit(a.ctx, "update:progress", map[string]any{
			"percent": float64(done) / float64(total),
			"stage":   stage,
		})
	}
	return update.DownloadAndStage(ctx, info, staging, progress, nil)
}

// InstallUpdate launches the replacer helper and quits so the update can apply.
func (a *App) InstallUpdate(stagedApp string) error {
	target, err := update.InstallTarget()
	if err != nil {
		return err
	}
	if err := update.LaunchReplacer(target, stagedApp); err != nil {
		return err
	}
	wruntime.Quit(a.ctx)
	return nil
}
