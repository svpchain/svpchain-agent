package runlog

import (
	"bufio"
	"encoding/json"
	"os"
)

// ReadAll loads every run from the JSONL log file (newest last).
func ReadAll(path string) ([]Run, error) {
	if path == "" {
		path = LogPath()
	}
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var runs []Run
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var run Run
		if err := json.Unmarshal(line, &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	return runs, sc.Err()
}

const (
	defaultRecentLimit = 100
	maxRecentLimit     = 200
)

// ClampRecentLimit bounds a GUI request. Zero or negative uses the default;
// values above maxRecentLimit are capped so a large JSONL cannot flood IPC.
func ClampRecentLimit(n int) int {
	if n <= 0 {
		return defaultRecentLimit
	}
	if n > maxRecentLimit {
		return maxRecentLimit
	}
	return n
}

// ReadRecent returns up to n most recent runs, oldest first (file order).
// n <= 0 returns every run.
func ReadRecent(n int) ([]Run, error) {
	runs, err := ReadAll("")
	if err != nil || n <= 0 || len(runs) <= n {
		return runs, err
	}
	return runs[len(runs)-n:], nil
}

// ReadRecentNewestFirst returns up to n most recent runs, newest first.
func ReadRecentNewestFirst(n int) ([]Run, error) {
	runs, err := ReadRecent(n)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}
	return runs, nil
}
