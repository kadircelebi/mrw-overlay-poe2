package app

import (
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"golang.org/x/sys/windows"
)

func TestToastsGoOutOneAtATimeFromOneThread(t *testing.T) {
	var mu sync.Mutex
	threads := map[uint32]bool{}
	var sent []string
	done := make(chan struct{}, 64)
	q := newToastQueue(func(opt notifications.NotificationOptions) {
		mu.Lock()
		threads[windows.GetCurrentThreadId()] = true
		sent = append(sent, opt.ID)
		mu.Unlock()
		done <- struct{}{}
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.Push(notifications.NotificationOptions{ID: "search-" + string(rune('a'+i))})
			q.Push(notifications.NotificationOptions{ID: "search-" + string(rune('a'+i))}) // too soon: dropped
		}(i)
	}
	wg.Wait()
	for i := 0; i < 8; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d toasts sent", i)
		}
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 8 {
		t.Errorf("sent %d toasts, want 8 (one per id)", len(sent))
	}
	if len(threads) != 1 {
		t.Errorf("toasts came from %d threads, want 1", len(threads))
	}
}
