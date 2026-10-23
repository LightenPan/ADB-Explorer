package store

import (
	"encoding/json"
	"os"
)

// TxTarget names a file a transaction backs up before changing it.
type TxTarget struct {
	// Kind is "manifest", "trashcan", "state", "config", "apply-history"
	// or "entry".
	Kind string
	// ID is the entry's id, for "entry".
	ID string
}

func (t TxTarget) path(p Paths) string {
	switch t.Kind {
	case "manifest":
		return p.ManifestFile
	case "trashcan":
		return p.TrashcanFile
	case "state":
		return p.StateFile
	case "config":
		return p.ConfigFile
	case "apply-history":
		return p.HistoriesDir + string(os.PathSeparator) + "system-hosts.json"
	case "entry":
		return p.EntriesDir + string(os.PathSeparator) + t.ID + ".hosts"
	}
	return ""
}

type txBackup struct {
	Path     string `json:"path"`
	Existed  bool   `json:"existed"`
	Contents []byte `json:"contents,omitempty"`
}

type txJournal struct {
	Version int        `json:"version"`
	Files   []txBackup `json:"files"`
}

func txPath(p Paths) string {
	return p.Internal + string(os.PathSeparator) + "storage-transaction.json"
}

// RecoverTx rolls back a transaction a crash interrupted.
func RecoverTx(p Paths) error {
	path := txPath(p)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var journal txJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return err
	}
	if journal.Version != 1 || len(journal.Files) == 0 {
		return nil
	}
	for _, file := range journal.Files {
		if file.Existed {
			if err := AtomicWrite(file.Path, file.Contents); err != nil {
				return err
			}
		} else if err := os.Remove(file.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Remove(path)
}

// Tx runs action with an undo journal, so that a failure or a crash during
// it restores every target.
func Tx(p Paths, targets []TxTarget, action func() error) error {
	if err := RecoverTx(p); err != nil {
		return err
	}
	seen := map[string]bool{}
	journal := txJournal{Version: 1}
	for _, target := range targets {
		path := target.path(p)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				journal.Files = append(journal.Files, txBackup{Path: path})
				continue
			}
			return err
		}
		journal.Files = append(journal.Files, txBackup{Path: path, Existed: true, Contents: data})
	}
	if len(journal.Files) == 0 {
		return action()
	}
	body, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	path := txPath(p)
	if err := AtomicWrite(path, body); err != nil {
		return err
	}
	if err := action(); err != nil {
		_ = RecoverTx(p)
		return err
	}
	return os.Remove(path)
}
