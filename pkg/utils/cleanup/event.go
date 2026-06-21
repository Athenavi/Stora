package cleanup

import (
	"log"
	"sync"
)

// CleanupEvent carries data about an expired/cancelled upload that needs cleanup.
type CleanupEvent struct {
	UploadID  string
	UserID    int64
	TempDir   string // e.g. "temp/upload/{upload_id}/"
}

// CleanupObserver is the interface that cleanup subscribers must implement.
type CleanupObserver interface {
	// OnCleanup is called when a CleanupEvent is fired.
	// Implementations should be idempotent — they may be called multiple times
	// for the same event (e.g. manual cancel + scheduled sweep).
	OnCleanup(event CleanupEvent) error
}

// CleanupNotifier manages observers and broadcasts cleanup events.
type CleanupNotifier struct {
	mu        sync.RWMutex
	observers []CleanupObserver
}

// NewCleanupNotifier creates a new CleanupNotifier.
func NewCleanupNotifier() *CleanupNotifier {
	return &CleanupNotifier{}
}

// Register adds an observer to receive cleanup events.
func (n *CleanupNotifier) Register(o CleanupObserver) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.observers = append(n.observers, o)
}

// Fire sends a cleanup event to all registered observers.
// Errors are logged but not returned — one observer failure does not block others.
func (n *CleanupNotifier) Fire(event CleanupEvent) {
	n.mu.RLock()
	observers := make([]CleanupObserver, len(n.observers))
	copy(observers, n.observers)
	n.mu.RUnlock()

	for _, o := range observers {
		if err := o.OnCleanup(event); err != nil {
			log.Printf("[Cleanup] observer %T failed for upload %s: %v", o, event.UploadID, err)
		}
	}
}
