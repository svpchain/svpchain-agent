package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const (
	sessionsDirName = "sessions"
	blobsDirName    = "blobs"
	indexFileName   = "index.json"

	// toolResultKeepLen is the max tool-result length kept in context; longer
	// results are archived as blobs and truncated in the transcript (projection).
	toolResultKeepLen = 4000

	titleMaxLen = 40
)

var reKeyHex = regexp.MustCompile(`(?i)(0x)?[0-9a-f]{64}`)

var (
	dirOverride string
	sharedOnce  sync.Once
	sharedStore *Store
)

// SetDirOverride redirects the sessions directory for tests.
func SetDirOverride(dir string) {
	dirOverride = dir
	sharedOnce = sync.Once{}
	sharedStore = nil
}

// Dir returns the sessions directory (next to prefs.json), or "" when unknown.
func Dir() string {
	if dirOverride != "" {
		return dirOverride
	}
	p := prefs.Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), sessionsDirName)
}

// Entry is one persisted conversation message.
type Entry struct {
	At      time.Time   `json:"at"`
	RunID   string      `json:"run_id,omitempty"`
	Msg     llm.Message `json:"msg"`
	FullRef string      `json:"full_ref,omitempty"`
}

// SessionInfo describes one conversation in the index.
type SessionInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ChainID   string    `json:"chain_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  int       `json:"messages"`
}

type indexFile struct {
	Current  string        `json:"current,omitempty"`
	Sessions []SessionInfo `json:"sessions"`
}

// summaryState is the persisted compaction state for one session.
type summaryState struct {
	// Upto is the number of leading JSONL entries covered by Text.
	Upto      int       `json:"upto"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists conversations under the sessions directory.
type Store struct {
	mu  sync.Mutex
	dir string
}

// Shared returns the process-wide store singleton.
func Shared() *Store {
	sharedOnce.Do(func() {
		sharedStore = &Store{dir: Dir()}
	})
	return sharedStore
}

// Enabled reports whether the store has a usable directory.
func (s *Store) Enabled() bool {
	return s != nil && s.dir != ""
}

func (s *Store) indexPath() string { return filepath.Join(s.dir, indexFileName) }
func (s *Store) jsonlPath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}
func (s *Store) summaryPath(id string) string {
	return filepath.Join(s.dir, id+".summary.json")
}

func (s *Store) loadIndexLocked() indexFile {
	var idx indexFile
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		return idx
	}
	_ = json.Unmarshal(data, &idx)
	return idx
}

func (s *Store) saveIndexLocked(idx indexFile) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o600)
}

// List returns sessions, most recently updated first.
func (s *Store) List() []SessionInfo {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	out := append([]SessionInfo(nil), idx.Sessions...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Current returns the active session, if any.
func (s *Store) Current() (SessionInfo, bool) {
	if !s.Enabled() {
		return SessionInfo{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	if idx.Current == "" {
		return SessionInfo{}, false
	}
	for _, sess := range idx.Sessions {
		if sess.ID == idx.Current {
			return sess, true
		}
	}
	return SessionInfo{}, false
}

// SetCurrent switches the active session.
func (s *Store) SetCurrent(id string) error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	for _, sess := range idx.Sessions {
		if sess.ID == id {
			idx.Current = id
			return s.saveIndexLocked(idx)
		}
	}
	return fmt.Errorf("unknown session %q", id)
}

// Create starts a new session and makes it current.
func (s *Store) Create(chainID string) (SessionInfo, error) {
	if !s.Enabled() {
		return SessionInfo{}, fmt.Errorf("history store disabled (no config dir)")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	sess := SessionInfo{
		ID:        uuid.NewString(),
		ChainID:   strings.TrimSpace(chainID),
		CreatedAt: now,
		UpdatedAt: now,
	}
	idx := s.loadIndexLocked()
	idx.Sessions = append(idx.Sessions, sess)
	idx.Current = sess.ID
	if err := s.saveIndexLocked(idx); err != nil {
		return SessionInfo{}, err
	}
	return sess, nil
}

// Delete removes a session and its files.
func (s *Store) Delete(id string) error {
	if !s.Enabled() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.loadIndexLocked()
	kept := idx.Sessions[:0]
	for _, sess := range idx.Sessions {
		if sess.ID != id {
			kept = append(kept, sess)
		}
	}
	idx.Sessions = kept
	if idx.Current == id {
		idx.Current = ""
	}
	if err := s.saveIndexLocked(idx); err != nil {
		return err
	}
	_ = os.Remove(s.jsonlPath(id))
	_ = os.Remove(s.summaryPath(id))
	return nil
}

// Append persists messages from one run, applying projection to large tool
// results and key-redaction to user messages.
func (s *Store) Append(id, runID string, msgs []llm.Message) error {
	if !s.Enabled() || id == "" || len(msgs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.jsonlPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	now := time.Now().UTC()
	var firstUser string
	for i, m := range msgs {
		e := Entry{At: now, RunID: runID, Msg: m}
		switch m.Role {
		case "user":
			e.Msg.Content = redactKeys(m.Content)
			if firstUser == "" {
				firstUser = e.Msg.Content
			}
		case "tool":
			if len(m.Content) > toolResultKeepLen {
				ref, err := s.writeBlobLocked(id, runID, i, m.Content)
				if err == nil {
					e.FullRef = ref
				}
				e.Msg.Content = m.Content[:toolResultKeepLen] +
					"\n[truncated — full result archived locally]"
			}
		}
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}

	idx := s.loadIndexLocked()
	for i := range idx.Sessions {
		if idx.Sessions[i].ID != id {
			continue
		}
		idx.Sessions[i].UpdatedAt = now
		idx.Sessions[i].Messages += len(msgs)
		if idx.Sessions[i].Title == "" && firstUser != "" {
			idx.Sessions[i].Title = makeTitle(firstUser)
		}
		break
	}
	return s.saveIndexLocked(idx)
}

func (s *Store) writeBlobLocked(id, runID string, i int, content string) (string, error) {
	dir := filepath.Join(s.dir, blobsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d-%d.txt", id, time.Now().UnixNano(), i)
	if runID != "" {
		name = fmt.Sprintf("%s-%d.txt", runID, i)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return filepath.Join(blobsDirName, name), nil
}

// Entries loads all persisted entries of a session.
func (s *Store) Entries(id string) ([]Entry, error) {
	if !s.Enabled() || id == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entriesLocked(id)
}

func (s *Store) entriesLocked(id string) ([]Entry, error) {
	f, err := os.Open(s.jsonlPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (s *Store) loadSummaryLocked(id string) (summaryState, bool) {
	data, err := os.ReadFile(s.summaryPath(id))
	if err != nil {
		return summaryState{}, false
	}
	var sum summaryState
	if err := json.Unmarshal(data, &sum); err != nil || strings.TrimSpace(sum.Text) == "" {
		return summaryState{}, false
	}
	return sum, true
}

func (s *Store) saveSummaryLocked(id string, sum summaryState) error {
	sum.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.summaryPath(id), data, 0o600)
}

// Context returns the messages to prepend as conversation history: the
// compaction summary block (if any) followed by entries after the summary cut.
func (s *Store) Context(id string) ([]llm.Message, error) {
	if !s.Enabled() || id == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.entriesLocked(id)
	if err != nil {
		return nil, err
	}
	var msgs []llm.Message
	if sum, ok := s.loadSummaryLocked(id); ok && sum.Upto > 0 && sum.Upto <= len(entries) {
		// The ack keeps user/assistant roles alternating for the Anthropic path.
		msgs = append(msgs,
			llm.Message{Role: "user", Content: "## Summary of earlier conversation (auto-compacted)\n\n" + sum.Text},
			llm.Message{Role: "assistant", Content: "Understood — I have the earlier conversation context."},
		)
		entries = entries[sum.Upto:]
	}
	for _, e := range entries {
		msgs = append(msgs, e.Msg)
	}
	return msgs, nil
}

func makeTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > titleMaxLen {
		return string(r[:titleMaxLen]) + "…"
	}
	return s
}

// redactKeys replaces 64-hex sequences (private-key shaped) in user input.
func redactKeys(s string) string {
	return reKeyHex.ReplaceAllString(s, "[REDACTED_KEY]")
}
