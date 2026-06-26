package main

import (
	"sync"
	"time"
)

// pendingStore mirrors the signed scheduling pending-action server-side, keyed by
// conversation_id. The chatbot UI round-trips the pending action through a hidden
// form field, but that client round-trip proved unreliable across agent switches
// (a general-agent turn or an HTMX out-of-band swap could drop it), which silently
// broke confirmations: a "yes" reached the general agent, no write happened, and
// the model sometimes fabricated a "scheduled" reply. conversation_id round-trips
// reliably (it drives conversation logging), so mirroring the pending action here
// makes confirmation independent of the fragile client field.
//
// In-memory and single-process: entries expire with the same TTL as the signed
// action and do not survive a restart — which is fine, since a restart also
// rotates the ephemeral signing secret and invalidates any outstanding
// confirmation anyway. The signature is still the security boundary; this store
// only improves delivery, it does not bypass verification.
var pendingStore = struct {
	sync.Mutex
	m map[int64]pendingEntry
}{m: map[int64]pendingEntry{}}

type pendingEntry struct {
	pa        *PendingAction
	expiresAt time.Time
}

// storePending records (or refreshes) the pending action for a conversation.
// No-ops for an unknown conversation (id 0) or a nil action.
func storePending(conversationID int64, pa *PendingAction, now time.Time) {
	if conversationID == 0 || pa == nil {
		return
	}
	pendingStore.Lock()
	defer pendingStore.Unlock()
	pendingStore.m[conversationID] = pendingEntry{pa: pa, expiresAt: now.Add(pendingActionTTL)}
}

// loadPending returns the live pending action for a conversation, or nil if none
// is stored or it has expired (expired entries are evicted on read).
func loadPending(conversationID int64, now time.Time) *PendingAction {
	if conversationID == 0 {
		return nil
	}
	pendingStore.Lock()
	defer pendingStore.Unlock()
	e, ok := pendingStore.m[conversationID]
	if !ok {
		return nil
	}
	if now.After(e.expiresAt) {
		delete(pendingStore.m, conversationID)
		return nil
	}
	return e.pa
}

// clearPending drops any stored pending action for a conversation — called once a
// write completes, a confirmation is cancelled, or the request is abandoned.
func clearPending(conversationID int64) {
	if conversationID == 0 {
		return
	}
	pendingStore.Lock()
	defer pendingStore.Unlock()
	delete(pendingStore.m, conversationID)
}
