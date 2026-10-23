package hostswrite

import (
	"sync"

	"github.com/oldj/switchhosts/internal/store"
)

// RecoveryStatus is the state of an apply that could not be confirmed.
type RecoveryStatus string

// Recovery states.
const (
	RecoveryNone    RecoveryStatus = ""
	RecoveryApplied RecoveryStatus = "applied"
	RecoveryUnknown RecoveryStatus = "unknown"
)

// RecoveryView is what the app shows about an unconfirmed apply.
type RecoveryView struct {
	Status string           `json:"status"`
	List   []store.NodeJSON `json:"list,omitempty"`
}

type pendingApply struct {
	view     RecoveryView
	expected string
}

// Recovery keeps the state of an apply whose metadata save failed, outside
// the windows so closing them loses nothing.
type Recovery struct {
	mu          sync.Mutex
	pending     *pendingApply
	beforeApply *pendingApply
	requested   bool
	applyMu     sync.Mutex
}

// LockApply serialises applies.
func (r *Recovery) LockApply() func() {
	r.applyMu.Lock()
	return r.applyMu.Unlock
}

// Begin marks an apply as started and unconfirmed.
func (r *Recovery) Begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeApply = r.pending
	r.pending = &pendingApply{view: RecoveryView{Status: string(RecoveryUnknown)}}
}

// ApplyFailed restores the state from before an apply that did not happen.
func (r *Recovery) ApplyFailed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = r.beforeApply
	r.beforeApply = nil
}

// Record notes an apply whose metadata could not be saved.
func (r *Recovery) Record(list []store.NodeJSON, expected string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeApply = nil
	r.pending = &pendingApply{
		view:     RecoveryView{Status: string(RecoveryApplied), List: list},
		expected: expected,
	}
}

// Request asks the main window to reconcile the pending apply.
func (r *Recovery) Request() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requested = r.pending != nil
	return r.requested
}

// TakeRequest consumes a pending request.
func (r *Recovery) TakeRequest() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	requested := r.requested
	r.requested = false
	return requested && r.pending != nil
}

// Pending reports whether an apply awaits confirmation.
func (r *Recovery) Pending() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending != nil
}

// Snapshot returns the current view, checking that the system file still
// holds what the apply wrote.
func (r *Recovery) Snapshot() *RecoveryView {
	r.mu.Lock()
	pending := r.pending
	r.mu.Unlock()
	if pending == nil {
		return nil
	}
	if pending.view.Status == string(RecoveryApplied) && !SystemHostsMatches(pending.expected) {
		r.mu.Lock()
		if r.pending != nil {
			r.pending.view = RecoveryView{Status: string(RecoveryUnknown)}
		}
		r.mu.Unlock()
		return &RecoveryView{Status: string(RecoveryUnknown)}
	}
	view := pending.view
	return &view
}

// Finish clears the pending apply when the file still holds expected.
func (r *Recovery) Finish(expected string) *RecoveryView {
	current := SystemHostsMatches(expected)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeApply = nil
	if current {
		r.pending = nil
		return nil
	}
	r.pending = &pendingApply{view: RecoveryView{Status: string(RecoveryUnknown)}}
	view := r.pending.view
	return &view
}

// Restored records the outcome of a compensation.
func (r *Recovery) Restored(previous string) *RecoveryView {
	current := SystemHostsMatches(previous)
	r.mu.Lock()
	previousView := r.beforeApply
	r.beforeApply = nil
	if current {
		r.pending = previousView
	} else {
		r.pending = &pendingApply{view: RecoveryView{Status: string(RecoveryUnknown)}}
	}
	pending := r.pending
	r.mu.Unlock()
	if pending == nil {
		return nil
	}
	view := pending.view
	return &view
}
