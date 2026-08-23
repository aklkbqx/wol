package presence

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheSharesConcurrentProbe(t *testing.T) {
	cache := NewCache(time.Minute)
	var calls atomic.Int32
	probe := func(context.Context) Result {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return Result{DeviceID: "device-1", Status: StatusOnline, Method: MethodTCPVerify}
	}

	results := make(chan Result, 2)
	go func() {
		result, err := cache.Resolve(context.Background(), "device-1:3389", false, probe)
		if err != nil {
			t.Errorf("first resolve failed: %v", err)
		}
		results <- result
	}()
	go func() {
		result, err := cache.Resolve(context.Background(), "device-1:3389", false, probe)
		if err != nil {
			t.Errorf("second resolve failed: %v", err)
		}
		results <- result
	}()

	<-results
	<-results
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one probe, got %d", got)
	}

	result, ok := cache.Get("device-1:3389", false)
	if !ok || !result.Cached || result.ExpiresAt == "" {
		t.Fatalf("expected cached result, got %+v (ok=%v)", result, ok)
	}
}

func TestCacheForceBypassesFreshEntry(t *testing.T) {
	cache := NewCache(time.Minute)
	cache.Set("device-1:3389", Result{DeviceID: "device-1", Status: StatusOffline})
	var calls atomic.Int32
	result, err := cache.Resolve(context.Background(), "device-1:3389", true, func(context.Context) Result {
		calls.Add(1)
		return Result{DeviceID: "device-1", Status: StatusOnline}
	})
	if err != nil || result.Status != StatusOnline || calls.Load() != 1 {
		t.Fatalf("force resolve did not bypass cache: result=%+v err=%v calls=%d", result, err, calls.Load())
	}
}
