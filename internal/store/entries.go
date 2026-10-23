package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Entries reads and writes the content files of local and remote hosts.
// Content is always stored with LF line endings.
func entryPath(p Paths, id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", &PathError{Path: id}
	}
	return filepath.Join(p.EntriesDir, id+".hosts"), nil
}

// PathError reports an unsafe entry id.
type PathError struct{ Path string }

func (e *PathError) Error() string { return "illegal entry id: " + e.Path }

// ReadEntry returns a node's content, "" when it has none.
func ReadEntry(p Paths, id string) (string, error) {
	path, err := entryPath(p, id)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return NormalizeToLF(string(data)), nil
}

// WriteEntry stores a node's content.
func WriteEntry(p Paths, id, content string) error {
	path, err := entryPath(p, id)
	if err != nil {
		return err
	}
	return AtomicWrite(path, []byte(NormalizeToLF(content)))
}

// DeleteEntry removes a node's content file.
func DeleteEntry(p Paths, id string) error {
	path, err := entryPath(p, id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// TrashItem is an entry in trashcan.json.
type TrashItem struct {
	Data      NodeJSON `json:"data"`
	AddTimeMS int64    `json:"add_time_ms"`
	ParentID  *string  `json:"parent_id"`
}

// Trashcan is trashcan.json.
type Trashcan struct {
	Format        string      `json:"format"`
	SchemaVersion int         `json:"schemaVersion"`
	Items         []TrashItem `json:"items"`
}

// LoadTrashcan reads trashcan.json.
func LoadTrashcan(p Paths) (*Trashcan, error) {
	data, err := os.ReadFile(p.TrashcanFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Trashcan{Format: "switchhosts-trashcan", SchemaVersion: 1, Items: []TrashItem{}}, nil
		}
		return nil, err
	}
	var t Trashcan
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if t.Items == nil {
		t.Items = []TrashItem{}
	}
	t.Format, t.SchemaVersion = "switchhosts-trashcan", 1
	return &t, nil
}

// SaveTrashcan writes trashcan.json.
func SaveTrashcan(p Paths, t *Trashcan) error {
	body, err := marshalIndentJSON(map[string]any{
		"format":        "switchhosts-trashcan",
		"schemaVersion": 1,
		"items":         t.Items,
	})
	if err != nil {
		return err
	}
	return AtomicWrite(p.TrashcanFile, body)
}

// RemoveItem removes the trash item of a node id.
func (t *Trashcan) RemoveItem(id string) (TrashItem, bool) {
	for i, item := range t.Items {
		if NodeID(item.Data) == id {
			t.Items = append(t.Items[:i:i], t.Items[i+1:]...)
			return item, true
		}
	}
	return TrashItem{}, false
}

// StateFile is internal/state.json: per-machine UI state.
type StateFile struct {
	Collapsed  []string  `json:"collapsedNodeIds"`
	WindowMain *Geometry `json:"windowMain,omitempty"`
}

// Geometry is a saved window position and size.
type Geometry struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized,omitempty"`
}

// LoadState reads internal/state.json.
func LoadState(p Paths) *StateFile {
	state := &StateFile{Collapsed: []string{}}
	data, err := os.ReadFile(p.StateFile)
	if err != nil {
		return state
	}
	var raw struct {
		Tree struct {
			CollapsedNodeIDs []string `json:"collapsedNodeIds"`
		} `json:"tree"`
		Window struct {
			Main *Geometry `json:"main"`
		} `json:"window"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return state
	}
	if raw.Tree.CollapsedNodeIDs != nil {
		state.Collapsed = raw.Tree.CollapsedNodeIDs
	}
	state.WindowMain = raw.Window.Main
	return state
}

// Save writes internal/state.json.
func (s *StateFile) Save(p Paths) error {
	body, err := marshalIndentJSON(map[string]any{
		"format":        "switchhosts-state",
		"schemaVersion": 1,
		"tree":          map[string]any{"collapsedNodeIds": s.Collapsed},
		"window":        map[string]any{"main": s.WindowMain},
	})
	if err != nil {
		return err
	}
	return AtomicWrite(p.StateFile, body)
}
