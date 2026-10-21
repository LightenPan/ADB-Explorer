package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"adbexplorer/internal/adb"
)

type treeNode struct {
	path     string
	name     string
	children []*treeNode
	open     bool
	loaded   bool
	loading  bool
	gen      int
}

var treeRoot *treeNode

var treeGen int

type treeRow struct {
	Path, Name    string
	Depth         int
	Open, Loading bool
}

// treeAt ensures the root exists and returns the node for path, creating
// missing segments along the way. It does not fetch.
func (a *app) treeAt(path string) *treeNode {
	if treeRoot == nil {
		treeRoot = &treeNode{path: "/", name: "root"}
	}
	path = cleanPath(path)
	if path == "/" {
		return treeRoot
	}
	node := treeRoot
	cur := "/"
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if seg == "" {
			continue
		}
		cur = joinPath(cur, seg)
		var child *treeNode
		for _, c := range node.children {
			if c.name == seg {
				child = c
				break
			}
		}
		if child == nil {
			child = &treeNode{path: cur, name: seg}
			node.children = append(node.children, child)
		}
		node = child
	}
	return node
}

// toggleTree flips a node open or closed. Opening an unloaded node lists the
// directory in the background. A newer toggle for the same node wins.
func (a *app) toggleTree(path string) {
	node := a.treeAt(path)
	node.open = !node.open
	if !node.open || node.loaded || node.loading {
		return
	}
	node.loading = true
	treeGen++
	gen := treeGen
	node.gen = gen
	serial := a.serial
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		entries, err := a.client().List(ctx, serial, node.path)
		a.winUpdate(func() {
			// A newer toggle owns the node; leave its loading flag alone.
			if node.gen != gen {
				return
			}
			node.loading = false
			if a.serial != serial || err != nil {
				return
			}
			kids := dirNodes(entries)
			for _, c := range kids {
				c.path = joinPath(node.path, c.name)
			}
			node.children = kids
			node.loaded = true
		})
	}()
}

// dirNodes keeps directories and sorts them by name, case-insensitively.
func dirNodes(entries []adb.Entry) []*treeNode {
	var nodes []*treeNode
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		nodes = append(nodes, &treeNode{name: e.Name})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return strings.ToLower(nodes[i].name) < strings.ToLower(nodes[j].name)
	})
	return nodes
}

// treeRows is a depth-first listing of open nodes, root included. Closed
// nodes are listed but their children are not. At most 300 rows.
func treeRows() []treeRow {
	if treeRoot == nil {
		return nil
	}
	var rows []treeRow
	var walk func(n *treeNode, depth int)
	walk = func(n *treeNode, depth int) {
		if len(rows) >= 300 {
			return
		}
		rows = append(rows, treeRow{
			Path:    n.path,
			Name:    n.name,
			Depth:   depth,
			Open:    n.open,
			Loading: n.loading,
		})
		if !n.open {
			return
		}
		for _, c := range n.children {
			if len(rows) >= 300 {
				return
			}
			walk(c, depth+1)
		}
	}
	walk(treeRoot, 0)
	return rows
}

// resetTree drops the cached folder tree.
func (a *app) resetTree() { treeRoot = nil }
