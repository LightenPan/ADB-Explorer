package hostswrite

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/oldj/switchhosts/internal/store"
)

// HistoryItem is one saved system hosts snapshot.
type HistoryItem struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	AddTimeMS int64  `json:"add_time_ms"`
	Label     string `json:"label,omitempty"`
}

// HistoriesPath is the apply history file.
func HistoriesPath(p store.Paths) string {
	return filepath.Join(p.HistoriesDir, "system-hosts.json")
}

// LoadHistory reads the apply history.
func LoadHistory(path string) ([]HistoryItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []HistoryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []HistoryItem{}
	}
	return items, nil
}

// SaveHistory writes the apply history.
func SaveHistory(path string, items []HistoryItem) error {
	if items == nil {
		items = []HistoryItem{}
	}
	body, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return store.AtomicWrite(path, append(body, '\n'))
}

var historySeq atomic.Uint64

// RecordChange appends the snapshots of an apply to the history.
func RecordChange(p store.Paths, limit int, enabled bool, previous, next string) error {
	if !enabled || previous == next {
		return nil
	}
	path := HistoriesPath(p)
	items, err := LoadHistory(path)
	if err != nil {
		return err
	}
	now := store.NowMillis()
	for _, content := range []string{previous, next} {
		if len(items) > 0 && items[len(items)-1].Content == content {
			continue
		}
		items = append(items, HistoryItem{
			ID:        "apply_" + itoa(now) + "_" + itoa(int64(historySeq.Add(1))),
			Content:   content,
			AddTimeMS: now,
		})
	}
	items = trimHistory(items, limit)
	return SaveHistory(path, items)
}

func trimHistory(items []HistoryItem, limit int) []HistoryItem {
	if limit <= 0 {
		return items
	}
	if len(items) > limit {
		return items[len(items)-limit:]
	}
	return items
}

// EnforceHistoryLimit trims the history to a limit.
func EnforceHistoryLimit(path string, limit int) error {
	items, err := LoadHistory(path)
	if err != nil {
		return err
	}
	trimmed := trimHistory(items, limit)
	if len(trimmed) == len(items) {
		return nil
	}
	return SaveHistory(path, trimmed)
}

// DeleteHistoryItem removes one snapshot.
func DeleteHistoryItem(path, id string) (bool, error) {
	items, err := LoadHistory(path)
	if err != nil {
		return false, err
	}
	out := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		return false, nil
	}
	return true, SaveHistory(path, out)
}

// ClearHistory empties the apply history.
func ClearHistory(path string) error {
	return SaveHistory(path, nil)
}

// LimitConfirmation carries what the user confirmed retiring.
type LimitConfirmation struct {
	PreviousLimit int `json:"previous_limit"`
	DeleteCount   int `json:"delete_count"`
}

// LimitResult is what changing the retention limit would do.
type LimitResult struct {
	ConfirmationRequired bool `json:"confirmation_required"`
	PreviousLimit        int  `json:"previous_limit"`
	Limit                int  `json:"limit"`
	DeleteCount          int  `json:"delete_count"`
	RetainedCount        int  `json:"retained_count"`
}

// UpdateLimit changes the history limit, asking first when snapshots go.
func UpdateLimit(p store.Paths, cfg *store.Config, limit int, confirmation *LimitConfirmation) (LimitResult, error) {
	path := HistoriesPath(p)
	items, err := LoadHistory(path)
	if err != nil {
		return LimitResult{}, err
	}
	remaining := items
	deleteCount := 0
	if limit > 0 && len(items) > limit {
		deleteCount = len(items) - limit
		remaining = items[deleteCount:]
	}
	confirmed := confirmation != nil &&
		confirmation.PreviousLimit == cfg.HistoryLimit &&
		confirmation.DeleteCount == deleteCount
	result := LimitResult{
		ConfirmationRequired: deleteCount > 0 && !confirmed,
		PreviousLimit:        cfg.HistoryLimit,
		Limit:                limit,
		DeleteCount:          deleteCount,
		RetainedCount:        len(remaining),
	}
	if result.ConfirmationRequired {
		return result, nil
	}
	cfg.HistoryLimit = limit
	targets := []store.TxTarget{{Kind: "config"}}
	if deleteCount > 0 {
		targets = append(targets, store.TxTarget{Kind: "apply-history"})
	}
	err = store.Tx(p, targets, func() error {
		if err := cfg.Save(p); err != nil {
			return err
		}
		if deleteCount > 0 {
			return SaveHistory(path, remaining)
		}
		return nil
	})
	return result, err
}

// CommandRunResult records a post-apply command run.
type CommandRunResult struct {
	ID        string `json:"_id"`
	Success   bool   `json:"success"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	AddTimeMS int64  `json:"add_time_ms"`
}

const (
	commandTimeout  = 30 * time.Second
	commandMaxItems = 200
)

// RunCommandHistoryPath is the post-apply command journal.
func RunCommandHistoryPath(p store.Paths) string {
	return filepath.Join(p.HistoriesDir, "cmd-after-apply.json")
}

// RunAfterApply runs the configured command as the signed-in user.
func RunAfterApply(cmd string) *CommandRunResult {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}
	now := store.NowMillis()
	result := &CommandRunResult{
		ID:        "cmd_" + itoa(now) + "_" + itoa(int64(historySeq.Add(1))),
		AddTimeMS: now,
	}
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.Command("cmd", "/d", "/s", "/c", cmd)
	} else {
		command = exec.Command("/bin/sh", "-c", cmd)
	}
	type output struct {
		data []byte
		err  error
	}
	done := make(chan output, 1)
	go func() {
		data, err := command.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				done <- output{data: exitErr.Stderr, err: err}
				return
			}
			done <- output{data: data, err: err}
			return
		}
		done <- output{data: data}
	}()
	select {
	case out := <-done:
		result.Success = out.err == nil
		result.Stdout = string(out.data)
		if out.err != nil {
			result.Stderr = out.err.Error()
		}
	case <-time.After(commandTimeout):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		result.Stderr = "command timed out after 30s"
	}
	return result
}

// LoadCommandHistory reads the post-apply command journal.
func LoadCommandHistory(path string) []CommandRunResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var items []CommandRunResult
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	return items
}

// SaveCommandHistory writes the post-apply command journal.
func SaveCommandHistory(path string, items []CommandRunResult) error {
	if items == nil {
		items = []CommandRunResult{}
	}
	body, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return store.AtomicWrite(path, append(body, '\n'))
}

// InsertCommandHistory appends a run and caps the journal.
func InsertCommandHistory(path string, item CommandRunResult) error {
	items := LoadCommandHistory(path)
	items = append(items, item)
	if len(items) > commandMaxItems {
		items = items[len(items)-commandMaxItems:]
	}
	return SaveCommandHistory(path, items)
}

// DeleteCommandHistoryItem removes one run.
func DeleteCommandHistoryItem(path, id string) bool {
	items := LoadCommandHistory(path)
	out := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		return false
	}
	_ = SaveCommandHistory(path, out)
	return true
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
