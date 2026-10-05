package app

import (
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// toastQueue sends every Windows notification from one OS thread.
//
// Wails pushes toasts through go-toast, which initialises the Windows
// Runtime (RoInitialize) once per process, on whichever thread happens to
// send the first toast, and then makes COM calls from whatever thread the
// caller is on. Live search sends a toast from its own goroutine for every
// listing; with a fast search several arrive on different threads and the
// process died with exit code 2 and no report (2026-09-27: with
// notifications off, 37 listings and no exit; on, it died after 2-7).
// Pinning all toasts to one locked thread keeps the COM work where it was
// initialised.
type toastQueue struct {
	send func(notifications.NotificationOptions)
	ch   chan notifications.NotificationOptions

	mu   sync.Mutex
	last map[string]time.Time
}

// minToastGap is the shortest time between two toasts with the same id; a
// busy live search would otherwise bury the screen in notifications.
const minToastGap = 5 * time.Second

func newToastQueue(send func(notifications.NotificationOptions)) *toastQueue {
	q := &toastQueue{
		send: send,
		ch:   make(chan notifications.NotificationOptions, 16),
		last: map[string]time.Time{},
	}
	go q.run()
	return q
}

func (q *toastQueue) run() {
	runtime.LockOSThread() // never unlocked: this thread belongs to toasts
	for opt := range q.ch {
		q.send(opt)
	}
}

// Push queues a toast. It never blocks: when the queue is full or the same
// id was shown moments ago, the toast is dropped.
func (q *toastQueue) Push(opt notifications.NotificationOptions) {
	now := time.Now()
	q.mu.Lock()
	if t, ok := q.last[opt.ID]; ok && now.Sub(t) < minToastGap {
		q.mu.Unlock()
		return
	}
	q.last[opt.ID] = now
	q.mu.Unlock()
	select {
	case q.ch <- opt:
	default:
		log.Printf("toast dropped, queue full: %s", opt.ID)
	}
}
