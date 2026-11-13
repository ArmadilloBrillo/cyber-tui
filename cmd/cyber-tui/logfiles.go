package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// keepLogs is how many debug-*.log and crash-*.log files survive pruning.
const keepLogs = 10

// logDir returns the directory for debug and crash logs, creating it. It
// falls back to the temp dir when the user cache dir is unavailable (e.g.
// $HOME unset in a container).
func logDir() (string, error) {
	bases := []string{os.TempDir()}
	if c, err := os.UserCacheDir(); err == nil {
		bases = append([]string{c}, bases...)
	}
	var err error
	for _, b := range bases {
		dir := filepath.Join(b, "cyber-tui")
		if err = os.MkdirAll(dir, 0o700); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("log dir: %w", err)
}

// runStamp names this run's log files. Colon-free for Windows; the PID keeps
// two instances started in the same second from sharing a file.
func runStamp() string {
	return time.Now().Format("2006-01-02T15-04-05") + "-" + strconv.Itoa(os.Getpid())
}

// createLog creates <dir>/<prefix>-<stamp>.log, failing rather than
// truncating if it somehow already exists.
func createLog(dir, prefix, stamp string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dir, prefix+"-"+stamp+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

// pruneLogs deletes all but the newest keep <prefix>-*.log files in dir.
// Stamps sort chronologically, so name order is age order.
func pruneLogs(dir, prefix string, keep int) {
	files, _ := filepath.Glob(filepath.Join(dir, prefix+"-*.log"))
	slices.Sort(files)
	for i := 0; i < len(files)-keep; i++ {
		os.Remove(files[i])
	}
}
