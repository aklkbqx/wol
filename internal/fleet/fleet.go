// Package fleet runs independent machine operations with bounded network work.
package fleet

import (
	"context"
	"sync"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
)

type Result struct {
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	SiteID    string `json:"siteId,omitempty"`
	Status    string `json:"status"`
	Method    string `json:"method,omitempty"`
	CheckedAt string `json:"checkedAt"`
	Message   string `json:"message,omitempty"`
}
type Operation func(context.Context, store.Device, time.Duration) Result

// Run owns all workers and closes the result channel. The bounded result buffer
// lets cancellation complete even when a UI has stopped consuming an old scan.
func Run(ctx context.Context, devices []store.Device, sites []store.Site, concurrency int, operation Operation) <-chan Result {
	if concurrency < 1 {
		concurrency = 16
	}
	if concurrency > 32 {
		concurrency = 32
	}
	results := make(chan Result, len(devices))
	global := make(chan struct{}, concurrency)
	grouped := make(map[string][]store.Device)
	settings := make(map[string]store.Site)
	for _, s := range sites {
		settings[s.ID] = s
	}
	for _, d := range devices {
		grouped[d.SiteID] = append(grouped[d.SiteID], d)
	}
	var wg sync.WaitGroup
	for id, items := range grouped {
		site := settings[id]
		limit := site.Concurrency
		if limit < 1 {
			limit = 4
		}
		if limit > 16 {
			limit = 16
		}
		if limit > len(items) {
			limit = len(items)
		}
		timeout := time.Duration(site.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 2500 * time.Millisecond
		}
		jobs := make(chan store.Device, len(items))
		for _, d := range items {
			jobs <- d
		}
		close(jobs)
		for i := 0; i < limit; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for d := range jobs {
					r := Result{DeviceID: d.ID, Name: d.Name, SiteID: d.SiteID, Status: "cancelled", CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
					if ctx.Err() == nil {
						select {
						case global <- struct{}{}:
							if ctx.Err() == nil {
								r = operation(ctx, d, timeout)
							}
							<-global
						case <-ctx.Done():
						}
					}
					results <- r
				}
			}()
		}
	}
	go func() { wg.Wait(); close(results) }()
	return results
}

func Check(detector *presence.Detector, profiles []store.RemoteProfile) Operation {
	byID := make(map[string]store.RemoteProfile)
	for _, p := range profiles {
		byID[p.DeviceID] = p
	}
	return func(ctx context.Context, d store.Device, timeout time.Duration) Result {
		port := d.VerifyPort
		if port == 0 {
			port = byID[d.ID].VerifyPort
		}
		r := detector.Probe(ctx, presence.Target{DeviceID: d.ID, IPAddress: d.IPAddress, VerifyPort: port}, timeout)
		return Result{DeviceID: d.ID, Name: d.Name, SiteID: d.SiteID, Status: string(r.Status), Method: string(r.Method), CheckedAt: r.CheckedAt, Message: r.Message}
	}
}
func Wake(service *wakeservice.Service) Operation {
	return func(ctx context.Context, d store.Device, _ time.Duration) Result {
		timed, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		result, err := service.WakeDevice(timed, d.ID, wakeservice.Options{Repeat: 3, Interval: 200 * time.Millisecond})
		r := Result{DeviceID: d.ID, Name: d.Name, SiteID: d.SiteID, Status: "sent", CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Message: result.Detail}
		if err != nil {
			r.Status = "failed"
			r.Message = err.Error()
		}
		return r
	}
}
