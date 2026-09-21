package fleet

import (
	"context"
	"fmt"
	"github.com/aklkbqx/wol/internal/store"
	"sync"
	"testing"
	"time"
)

func TestHundredMachinesBoundedAndStreamed(t *testing.T) {
	devices := make([]store.Device, 100)
	for i := range devices {
		devices[i] = store.Device{ID: fmt.Sprint(i), SiteID: fmt.Sprint(i % 3)}
	}
	sites := []store.Site{{ID: "0", Concurrency: 2}, {ID: "1", Concurrency: 2}, {ID: "2", Concurrency: 2}}
	var mu sync.Mutex
	active, total, maxTotal := map[string]int{}, 0, 0
	results := Run(t.Context(), devices, sites, 4, func(ctx context.Context, d store.Device, _ time.Duration) Result {
		mu.Lock()
		active[d.SiteID]++
		total++
		if total > maxTotal {
			maxTotal = total
		}
		if active[d.SiteID] > 2 || total > 4 {
			t.Errorf("limits exceeded")
		}
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		active[d.SiteID]--
		total--
		mu.Unlock()
		return Result{DeviceID: d.ID, Status: "online"}
	})
	seen := map[string]bool{}
	for r := range results {
		if seen[r.DeviceID] {
			t.Fatal("duplicate result")
		}
		seen[r.DeviceID] = true
	}
	if len(seen) != 100 || maxTotal < 2 {
		t.Fatalf("completed=%d peak=%d", len(seen), maxTotal)
	}
}
func TestCancellationDoesNotStartQueuedWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{}, 1)
	devices := make([]store.Device, 100)
	for i := range devices {
		devices[i].ID = fmt.Sprint(i)
	}
	results := Run(ctx, devices, []store.Site{{Concurrency: 1}}, 1, func(ctx context.Context, d store.Device, _ time.Duration) Result {
		started <- struct{}{}
		<-ctx.Done()
		return Result{DeviceID: d.ID, Status: "cancelled"}
	})
	<-started
	cancel()
	count := 0
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-results:
			if !ok {
				if count != 100 {
					t.Fatal(count)
				}
				return
			}
			count++
		case <-deadline:
			t.Fatal("workers did not stop")
		}
	}
}
func TestSlowSiteDoesNotBlockAnother(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	devices := []store.Device{{ID: "slow", SiteID: "a"}, {ID: "fast", SiteID: "b"}}
	results := Run(ctx, devices, nil, 2, func(ctx context.Context, d store.Device, _ time.Duration) Result {
		if d.ID == "slow" {
			<-ctx.Done()
		}
		return Result{DeviceID: d.ID}
	})
	select {
	case r := <-results:
		if r.DeviceID != "fast" {
			t.Fatal(r)
		}
	case <-time.After(time.Second):
		t.Fatal("fast site blocked")
	}
}
