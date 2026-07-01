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

// ReadRecent returns up to n most recent runs.
func ReadRecent(n int) ([]Run, error) {
	runs, err := ReadAll("")
	if err != nil || n <= 0 || len(runs) <= n {
		return runs, err
	}
	return runs[len(runs)-n:], nil
}
