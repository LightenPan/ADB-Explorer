package appui

import (
	"errors"
	"strings"

	"github.com/oldj/switchhosts/internal/hostswrite"
	"github.com/oldj/switchhosts/internal/store"
)

// Toggle switches an entry on or off, applying the resulting selection to
// the system hosts file.
func (a *App) Toggle(id string, on bool) error {
	if a.config.WriteMode == "" {
		a.openWriteModeDialog(dialogPending{ID: id, On: on})
		return nil
	}
	return a.applyToggle(id, on)
}

func (a *App) applyToggle(id string, on bool) error {
	if a.apply.Pending() {
		return errors.New(a.T("hosts_application_unknown"))
	}
	unlock := a.apply.LockApply()
	defer unlock()

	// Apply against the list the user is looking at, whose switches are
	// already in their new state.
	working := store.CloneList(a.list)
	store.SetOnState(&working, id, on, a.config.ChoiceMode, a.config.MultiChooseFolderAll)

	previous := a.list
	a.list = working
	if err := a.applyCurrentList(); err != nil {
		a.list = previous
		return err
	}
	if store.NodeID(a.Current()) == id {
		a.loadEditor()
		a.discardEditor()
	}
	return nil
}

// applyList reapplies the selection the app holds, as a refresh or an edit
// asks: the system hosts file takes the switched-on entries' content.
func (a *App) applyList() error {
	if a.apply.Pending() {
		return errors.New(a.T("hosts_application_unknown"))
	}
	unlock := a.apply.LockApply()
	defer unlock()
	return a.applyCurrentList()
}

// applyCurrentList writes a.list's selection to the system hosts file and
// saves it. The caller holds the apply lock.
func (a *App) applyCurrentList() error {
	content, err := hostswrite.Aggregate(a.list, a.paths, a.config.RemoveDuplicate)
	if err != nil {
		return err
	}

	a.apply.Begin()
	outcome, err := hostswrite.Apply(content, a.config.WriteMode,
		a.paths.HistoriesDir+"/hosts-encoding-backups")
	if err != nil {
		a.apply.ApplyFailed()
		return a.explainApplyError(err)
	}
	if !outcome.Unchanged {
		if historyErr := hostswrite.RecordChange(a.paths, a.config.HistoryLimit,
			a.config.HistoryEnabled, outcome.PreviousContent, outcome.NewContent); historyErr != nil {
			a.addToast(a.T("fail"), historyErr.Error(), true)
		}
	}
	if result := hostswrite.RunAfterApply(a.config.CmdAfterHostsApply); result != nil {
		_ = hostswrite.InsertCommandHistory(hostswrite.RunCommandHistoryPath(a.paths), *result)
	}

	// Persist the tree. A failure here leaves the system file changed, so
	// the recovery state asks the user to reapply.
	saved, err := store.LoadManifest(a.paths)
	if err != nil {
		a.apply.ApplyFailed()
		return err
	}
	if err := a.persistList(saved.Root); err != nil {
		if restoreErr := hostswrite.Restore(outcome.PreviousContent, outcome.NewContent); restoreErr == nil {
			a.apply.Restored(outcome.PreviousContent)
			return err
		}
		a.apply.Restored(outcome.PreviousContent)
		a.addToast(a.T("fail"), a.T("hosts_application_unknown"), true)
		return err
	}
	if view := a.apply.Finish(outcome.NewContent); view != nil {
		a.addToast(a.T("fail"), a.T("hosts_application_unknown"), true)
	}
	return nil
}

// explainApplyError turns a failed write into the message the user acts on:
// a missing right is the common cause, and the raw OS error means nothing to
// them.
func (a *App) explainApplyError(err error) error {
	var applyErr *hostswrite.ApplyError
	if !errors.As(err, &applyErr) {
		return err
	}
	switch applyErr.Code {
	case "no_access":
		// The rights are still missing, so the startup notice is due too.
		if a.permission == "" {
			hostsPath, _ := hostswrite.SystemHostsPath()
			a.permission = applyErr.Message
			a.permissionPath = hostsPath
			a.canElevate = hostswrite.CanElevate()
			a.elevated = hostswrite.Elevated()
		}
		return errors.New(a.T("apply_no_access"))
	case "cancelled":
		return errors.New(a.T("permission_cancelled"))
	}
	return err
}

// persistList saves the tree, refusing to overwrite a concurrent change.
func (a *App) persistList(expected []store.NodeJSON) error {
	current, err := store.LoadManifest(a.paths)
	if err != nil {
		return err
	}
	if !sameTree(current.Root, expected) {
		return store.ErrConflict
	}
	m := &store.Manifest{Root: a.list}
	return store.Tx(a.paths, []store.TxTarget{{Kind: "state"}, {Kind: "manifest"}}, func() error {
		return store.SaveManifest(a.paths, m)
	})
}

func sameTree(a, b []store.NodeJSON) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if store.NodeID(a[i]) != store.NodeID(b[i]) {
			return false
		}
	}
	return true
}

// SaveList persists the tree after an edit made in the interface.
func (a *App) SaveList() error {
	previous, err := store.LoadManifest(a.paths)
	if err != nil {
		return err
	}
	m := &store.Manifest{Root: a.list}
	return store.Tx(a.paths, []store.TxTarget{{Kind: "state"}, {Kind: "manifest"}}, func() error {
		if !sameTree(previous.Root, a.list) {
			// The list the interface shows replaced the saved one; this is
			// an edit, not a stale write, so the CAS check does not apply.
		}
		return store.SaveManifest(a.paths, m)
	})
}

// MoveToTrashcan moves entries to the trashcan.
func (a *App) MoveToTrashcan(ids []string) error {
	m, err := store.LoadManifest(a.paths)
	if err != nil {
		return err
	}
	t, err := store.LoadTrashcan(a.paths)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if node, parentID, ok := store.RemoveNode(&m.Root, id); ok {
			var parent *string
			if parentID != "" {
				parent = &parentID
			}
			t.Items = append(t.Items, store.TrashItem{Data: node, AddTimeMS: store.NowMillis(), ParentID: parent})
		}
	}
	err = store.Tx(a.paths, []store.TxTarget{{Kind: "state"}, {Kind: "manifest"}, {Kind: "trashcan"}}, func() error {
		if err := store.SaveManifest(a.paths, m); err != nil {
			return err
		}
		return store.SaveTrashcan(a.paths, t)
	})
	if err != nil {
		return err
	}
	a.list = m.Root
	a.trashcan = t.Items
	if a.InTrashcan(a.currentID) || store.FindNode(a.list, a.currentID) == nil {
		a.currentID = "0"
		a.loadEditor()
	}
	return nil
}

// RestoreFromTrashcan puts an entry back where it was.
func (a *App) RestoreFromTrashcan(id string) error {
	m, err := store.LoadManifest(a.paths)
	if err != nil {
		return err
	}
	t, err := store.LoadTrashcan(a.paths)
	if err != nil {
		return err
	}
	item, ok := t.RemoveItem(id)
	if !ok {
		return nil
	}
	parentID := ""
	if item.ParentID != nil {
		parentID = *item.ParentID
	}
	store.InsertNode(&m.Root, item.Data, parentID)
	err = store.Tx(a.paths, []store.TxTarget{{Kind: "state"}, {Kind: "manifest"}, {Kind: "trashcan"}}, func() error {
		if err := store.SaveManifest(a.paths, m); err != nil {
			return err
		}
		return store.SaveTrashcan(a.paths, t)
	})
	if err != nil {
		return err
	}
	a.list = m.Root
	a.trashcan = t.Items
	return nil
}

// DeleteFromTrashcan removes an entry for good, with its content.
func (a *App) DeleteFromTrashcan(id string) error {
	t, err := store.LoadTrashcan(a.paths)
	if err != nil {
		return err
	}
	item, ok := t.RemoveItem(id)
	if !ok {
		return nil
	}
	var ids []string
	store.CollectContentIDs([]store.NodeJSON{item.Data}, &ids)
	targets := []store.TxTarget{{Kind: "trashcan"}}
	for _, cid := range ids {
		targets = append(targets, store.TxTarget{Kind: "entry", ID: cid})
	}
	err = store.Tx(a.paths, targets, func() error {
		if err := store.SaveTrashcan(a.paths, t); err != nil {
			return err
		}
		for _, cid := range ids {
			if err := store.DeleteEntry(a.paths, cid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.trashcan = t.Items
	if a.currentID == id {
		a.currentID = "0"
		a.loadEditor()
	}
	return nil
}

// ClearTrashcan removes every trashed entry for good.
func (a *App) ClearTrashcan() error {
	t, err := store.LoadTrashcan(a.paths)
	if err != nil {
		return err
	}
	var ids []string
	for _, item := range t.Items {
		store.CollectContentIDs([]store.NodeJSON{item.Data}, &ids)
	}
	t.Items = nil
	targets := []store.TxTarget{{Kind: "trashcan"}}
	for _, cid := range ids {
		targets = append(targets, store.TxTarget{Kind: "entry", ID: cid})
	}
	err = store.Tx(a.paths, targets, func() error {
		if err := store.SaveTrashcan(a.paths, t); err != nil {
			return err
		}
		for _, cid := range ids {
			if err := store.DeleteEntry(a.paths, cid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.trashcan = nil
	return nil
}

// ToggleFolder opens or closes a folder in the list.
func (a *App) ToggleFolder(node store.NodeJSON) {
	collapsed := false
	if v, ok := node["is_collapsed"].(bool); ok {
		collapsed = v
	} else if open, ok := node["folder_open"].(bool); ok {
		collapsed = !open
	}
	node["is_collapsed"] = !collapsed
	node["folder_open"] = collapsed
	_ = store.SaveManifest(a.paths, &store.Manifest{Root: a.list})
}

// UpsertNode adds a new entry or replaces an edited one, then persists it.
func (a *App) UpsertNode(node store.NodeJSON, isNew bool) error {
	if isNew {
		id := newID()
		node["id"] = id
		if kind := store.NodeType(node); kind == "folder" {
			node["children"] = []store.NodeJSON{}
		}
		store.InsertNode(&a.list, node, "")
	} else {
		id := store.NodeID(node)
		existing := store.FindNode(a.list, id)
		if existing == nil {
			return store.ErrConflict
		}
		for key, value := range node {
			existing[key] = value
		}
	}
	return a.SaveList()
}

// CreateLocalEntry adds a local entry, the app's "New" action.
func (a *App) CreateLocalEntry() store.NodeJSON {
	node := store.NodeJSON{
		"id":    newID(),
		"title": a.T("untitled"),
		"type":  "local",
		"on":    false,
	}
	a.list = append(a.list, node)
	_ = store.WriteEntry(a.paths, store.NodeID(node), "")
	_ = a.SaveList()
	a.currentID = store.NodeID(node)
	a.loadEditor()
	return node
}

// NodeTitleText returns a node's title or the "untitled" label.
func (a *App) NodeTitleText(node store.NodeJSON) string {
	title := strings.TrimSpace(store.NodeTitle(node))
	if title == "" {
		return a.T("untitled")
	}
	return title
}

func newID() string {
	return nextUUID()
}
