package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Manifest is manifest.json: the hosts tree in the shape the app builds
// and edits.
type Manifest struct {
	Format        string     `json:"format"`
	SchemaVersion int        `json:"schemaVersion"`
	Root          []NodeJSON `json:"root"`
}

// LoadManifest reads manifest.json. A missing file is an empty tree.
func LoadManifest(p Paths) (*Manifest, error) {
	data, err := os.ReadFile(p.ManifestFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Format: "switchhosts-data", SchemaVersion: 1}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	if m.Root == nil {
		m.Root = []NodeJSON{}
	}
	m.Format, m.SchemaVersion = "switchhosts-data", 1
	collapsed := LoadState(p).Collapsed
	markCollapsed(m.Root, collapsed)
	return &m, nil
}

// SaveManifest writes manifest.json and the collapse state it carries.
func SaveManifest(p Paths, m *Manifest) error {
	collapsed := []string{}
	collectCollapsed(m.Root, &collapsed)
	state := LoadState(p)
	state.Collapsed = collapsed

	body, err := marshalIndentJSON(map[string]any{
		"format":        "switchhosts-data",
		"schemaVersion": 1,
		"root":          m.Root,
	})
	if err != nil {
		return err
	}
	if err := state.Save(p); err != nil {
		return err
	}
	return AtomicWrite(p.ManifestFile, body)
}

func markCollapsed(nodes []NodeJSON, collapsed []string) {
	set := make(map[string]bool, len(collapsed))
	for _, id := range collapsed {
		set[id] = true
	}
	var walk func([]NodeJSON)
	walk = func(nodes []NodeJSON) {
		for _, node := range nodes {
			if NodeType(node) == "folder" {
				id := NodeID(node)
				node["is_collapsed"] = set[id]
				node["folder_open"] = !set[id]
				walk(Children(node))
			}
		}
	}
	walk(nodes)
}

func collectCollapsed(nodes []NodeJSON, out *[]string) {
	for _, node := range nodes {
		if NodeType(node) != "folder" {
			continue
		}
		collapsed, _ := node["is_collapsed"].(bool)
		if open, ok := node["folder_open"].(bool); ok && !collapsed {
			collapsed = !open
		}
		if collapsed {
			*out = append(*out, NodeID(node))
		}
		collectCollapsed(Children(node), out)
	}
}

// NodeID returns a node's id.
func NodeID(node NodeJSON) string {
	id, _ := node["id"].(string)
	return id
}

// NodeType returns a node's type, "local" when it has none.
func NodeType(node NodeJSON) string {
	t, ok := node["type"].(string)
	if !ok || t == "" {
		return "local"
	}
	return t
}

// NodeTitle returns a node's title.
func NodeTitle(node NodeJSON) string {
	t, _ := node["title"].(string)
	return t
}

// NodeOn reports whether a node is switched on.
func NodeOn(node NodeJSON) bool {
	on, _ := node["on"].(bool)
	return on
}

// Children returns a folder's children.
func Children(node NodeJSON) []NodeJSON {
	raw, ok := node["children"].([]any)
	if !ok {
		if typed, ok := node["children"].([]NodeJSON); ok {
			return typed
		}
		return nil
	}
	out := make([]NodeJSON, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// SetChildren stores a folder's children.
func SetChildren(node NodeJSON, children []NodeJSON) {
	node["children"] = children
}

// CloneNode deep-copies a node.
func CloneNode(node NodeJSON) NodeJSON {
	body, err := json.Marshal(node)
	if err != nil {
		return node
	}
	var out NodeJSON
	if err := json.Unmarshal(body, &out); err != nil {
		return node
	}
	return out
}

// CloneList deep-copies a list of nodes.
func CloneList(nodes []NodeJSON) []NodeJSON {
	out := make([]NodeJSON, len(nodes))
	for i, node := range nodes {
		out[i] = CloneNode(node)
	}
	return out
}

// Flatten returns the nodes of the tree in display order.
func Flatten(nodes []NodeJSON) []NodeJSON {
	var out []NodeJSON
	var walk func([]NodeJSON)
	walk = func(nodes []NodeJSON) {
		for _, node := range nodes {
			out = append(out, node)
			walk(Children(node))
		}
	}
	walk(nodes)
	return out
}

// FindNode finds a node by id anywhere in the tree.
func FindNode(nodes []NodeJSON, id string) NodeJSON {
	for _, node := range Flatten(nodes) {
		if NodeID(node) == id {
			return node
		}
	}
	return nil
}

// FindParent finds the folder holding a node, nil at the top level.
func FindParent(nodes []NodeJSON, id string) NodeJSON {
	var walk func(nodes []NodeJSON) NodeJSON
	walk = func(nodes []NodeJSON) NodeJSON {
		for _, node := range nodes {
			for _, child := range Children(node) {
				if NodeID(child) == id {
					return node
				}
			}
			if found := walk(Children(node)); found != nil {
				return found
			}
		}
		return nil
	}
	return walk(nodes)
}

// RemoveNode removes a node by id and returns it, with its parent's id.
func RemoveNode(nodes *[]NodeJSON, id string) (NodeJSON, string, bool) {
	list := *nodes
	for i, node := range list {
		if NodeID(node) == id {
			*nodes = append(list[:i:i], list[i+1:]...)
			return node, "", true
		}
	}
	for _, node := range list {
		children := Children(node)
		if children == nil {
			continue
		}
		removed, _, ok := RemoveNode(&children, id)
		if ok {
			SetChildren(node, children)
			return removed, NodeID(node), true
		}
	}
	return nil, "", false
}

// InsertNode appends a node at the top level, or into the folder parentID.
func InsertNode(nodes *[]NodeJSON, node NodeJSON, parentID string) {
	if parentID != "" {
		if parent := findMutable(nodes, parentID); parent != nil && NodeType(parent) == "folder" {
			SetChildren(parent, append(Children(parent), node))
			return
		}
	}
	*nodes = append(*nodes, node)
}

func findMutable(nodes *[]NodeJSON, id string) NodeJSON {
	for _, node := range *nodes {
		if NodeID(node) == id {
			return node
		}
		if found := findMutableChildren(node, id); found != nil {
			return found
		}
	}
	return nil
}

func findMutableChildren(node NodeJSON, id string) NodeJSON {
	children := Children(node)
	if children == nil {
		return nil
	}
	for _, child := range children {
		if NodeID(child) == id {
			return child
		}
		if found := findMutableChildren(child, id); found != nil {
			return found
		}
	}
	return nil
}

// CollectContentIDs lists the ids of the local and remote nodes, which own
// content files.
func CollectContentIDs(nodes []NodeJSON, out *[]string) {
	for _, node := range nodes {
		switch NodeType(node) {
		case "local", "remote":
			*out = append(*out, NodeID(node))
		}
		CollectContentIDs(Children(node), out)
	}
}

// ErrConflict reports that the tree changed under a writer.
var ErrConflict = errors.New("the hosts list changed; reload it and retry")

// SetOnState applies a toggle the way the app's lists do: choice mode,
// folder cascades and parent reconciliation.
func SetOnState(root *[]NodeJSON, id string, on bool, defaultChoiceMode int, folderCascade bool) {
	path := indexPath(*root, id)
	if path == nil {
		return
	}
	node := nodeAt(*root, path)
	if node == nil {
		return
	}
	node["on"] = on
	inTopLevel := len(path) == 1

	if folderCascade {
		switchFolderChild(node, on)
		if !inTopLevel {
			reconcileParent(root, path, on)
		}
	}
	if !on {
		return
	}
	if inTopLevel {
		if defaultChoiceMode != 1 {
			return
		}
		for _, other := range *root {
			if NodeID(other) == id {
				continue
			}
			other["on"] = false
			if folderCascade {
				switchFolderChild(other, false)
			}
		}
		return
	}
	parentPath := path[:len(path)-1]
	parent := nodeAt(*root, parentPath)
	mode := defaultChoiceMode
	if parent != nil {
		if m, ok := intField(parent["folder_mode"]); ok && m != 0 {
			mode = m
		}
	}
	if mode != 1 {
		return
	}
	chosen := path[len(path)-1]
	if parent == nil {
		return
	}
	for i, child := range Children(parent) {
		if i == chosen {
			continue
		}
		child["on"] = false
		if folderCascade {
			switchFolderChild(child, false)
		}
	}
}

func intField(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func switchFolderChild(node NodeJSON, on bool) {
	if NodeType(node) != "folder" {
		return
	}
	if mode, ok := intField(node["folder_mode"]); ok && mode == 1 {
		return
	}
	for _, child := range Children(node) {
		child["on"] = on
		switchFolderChild(child, on)
	}
}

func reconcileParent(root *[]NodeJSON, path []int, on bool) {
	if len(path) < 2 {
		return
	}
	parentPath := path[:len(path)-1]
	parent := nodeAt(*root, parentPath)
	if parent == nil {
		return
	}
	if mode, ok := intField(parent["folder_mode"]); ok && mode == 1 {
		return
	}
	if !on {
		parent["on"] = false
	} else {
		all := true
		for _, child := range Children(parent) {
			if !NodeOn(child) {
				all = false
				break
			}
		}
		parent["on"] = all
	}
	reconcileParent(root, parentPath, on)
}

func indexPath(nodes []NodeJSON, id string) []int {
	for i, node := range nodes {
		if NodeID(node) == id {
			return []int{i}
		}
		if sub := indexPath(Children(node), id); sub != nil {
			return append([]int{i}, sub...)
		}
	}
	return nil
}

func nodeAt(nodes []NodeJSON, path []int) NodeJSON {
	if len(path) == 0 {
		return nil
	}
	node := nodes[path[0]]
	if node == nil {
		return nil
	}
	if len(path) == 1 {
		return node
	}
	return nodeAt(Children(node), path[1:])
}
