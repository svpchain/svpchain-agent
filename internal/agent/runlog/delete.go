package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Delete removes the run with the given id from the JSONL log.
func Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("run id is required")
	}
	persistMu.Lock()
	defer persistMu.Unlock()

	path := LogPath()
	runs, err := ReadAll(path)
	if err != nil {
		return err
	}
	kept := runs[:0]
	for _, run := range runs {
		if run.RunID != id {
			kept = append(kept, run)
		}
	}
	if len(kept) == len(runs) {
		return fmt.Errorf("run not found")
	}
	return writeAll(path, kept)
}

func patchRun(id string, fn func(*Run) error) (Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Run{}, errRunIDRequired
	}
	persistMu.Lock()
	defer persistMu.Unlock()

	path := LogPath()
	runs, err := ReadAll(path)
	if err != nil {
		return Run{}, err
	}
	idx := -1
	for i := range runs {
		if runs[i].RunID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Run{}, fmt.Errorf("run not found")
	}
	if err := fn(&runs[idx]); err != nil {
		return Run{}, err
	}
	if err := writeAll(path, runs); err != nil {
		return Run{}, err
	}
	return runs[idx], nil
}

// DeleteAll truncates the run log file.
func DeleteAll() error {
	persistMu.Lock()
	defer persistMu.Unlock()
	return writeAll(LogPath(), nil)
}

func writeAll(path string, runs []Run) error {
	if path == "" {
		return fmt.Errorf("run log path is not configured")
	}
	if len(runs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "agent_runs-*.jsonl.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, run := range runs {
		line, err := json.Marshal(run)
		if err != nil {
			_ = tmp.Close()
			return err
		}
		if _, err := tmp.Write(append(line, '\n')); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			return err
		}
	}
	ok = true
	return nil
}
