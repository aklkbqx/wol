package power

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDrivers(t *testing.T) {
	tests := []struct {
		platform string
		sudo     bool
		delay    time.Duration
		force    bool
		wantCmd  string
		cancel   string
	}{
		{
			platform: "windows",
			sudo:     false,
			delay:    0,
			force:    false,
			wantCmd:  "shutdown.exe /s /t 0",
			cancel:   "shutdown.exe /a",
		},
		{
			platform: "windows",
			sudo:     false,
			delay:    30 * time.Minute,
			force:    true,
			wantCmd:  "shutdown.exe /s /f /t 1800",
			cancel:   "shutdown.exe /a",
		},
		{
			platform: "linux",
			sudo:     true,
			delay:    0,
			force:    false,
			wantCmd:  "sudo shutdown -h now",
			cancel:   "sudo shutdown -c",
		},
		{
			platform: "linux",
			sudo:     false,
			delay:    15 * time.Minute,
			force:    false,
			wantCmd:  "shutdown -h +15",
			cancel:   "shutdown -c",
		},
		{
			platform: "darwin",
			sudo:     true,
			delay:    10 * time.Minute,
			force:    false,
			wantCmd:  "sudo shutdown -h +10",
			cancel:   "sudo killall shutdown",
		},
	}

	for _, tt := range tests {
		d := NewDriver(tt.platform, tt.sudo)
		gotCmd := d.BuildShutdownCommand(tt.delay, tt.force)
		if gotCmd != tt.wantCmd {
			t.Errorf("BuildShutdownCommand() for %s = %q, want %q", tt.platform, gotCmd, tt.wantCmd)
		}
		gotCancel := d.BuildCancelCommand()
		if gotCancel != tt.cancel {
			t.Errorf("BuildCancelCommand() for %s = %q, want %q", tt.platform, gotCancel, tt.cancel)
		}
	}
}

func TestServiceExecute(t *testing.T) {
	mockRunner := func(ctx context.Context, target Target, command string) (string, error) {
		if strings.Contains(target.Host, "fail") {
			return "host unreachable", errors.New("dial tcp: timeout")
		}
		return "command executed successfully", nil
	}

	svc := NewService(mockRunner)

	// Test immediate shutdown
	res, err := svc.Execute(context.Background(), Request{
		Target: Target{
			DeviceName: "test-pc",
			Host:       "192.168.8.200",
			User:       "aklkbqx",
			Platform:   "windows",
		},
		Force: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != ActionShutdownNow {
		t.Errorf("got action %s, want %s", res.Action, ActionShutdownNow)
	}
	if res.Command != "shutdown.exe /s /f /t 0" {
		t.Errorf("got command %q, want shutdown.exe /s /f /t 0", res.Command)
	}

	// Test scheduled shutdown
	resSched, err := svc.Execute(context.Background(), Request{
		Target: Target{
			DeviceName: "test-pc",
			Host:       "192.168.8.200",
			User:       "aklkbqx",
			Platform:   "windows",
		},
		Delay: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resSched.Action != ActionSchedule {
		t.Errorf("got action %s, want %s", resSched.Action, ActionSchedule)
	}
	if resSched.Command != "shutdown.exe /s /t 600" {
		t.Errorf("got command %q, want shutdown.exe /s /t 600", resSched.Command)
	}

	// Test cancel
	resCancel, err := svc.Execute(context.Background(), Request{
		Target: Target{
			DeviceName: "test-pc",
			Host:       "192.168.8.200",
			User:       "aklkbqx",
			Platform:   "windows",
		},
		Cancel: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resCancel.Action != ActionCancel {
		t.Errorf("got action %s, want %s", resCancel.Action, ActionCancel)
	}
	if resCancel.Command != "shutdown.exe /a" {
		t.Errorf("got command %q, want shutdown.exe /a", resCancel.Command)
	}

	// Test unsafe input
	_, err = svc.Execute(context.Background(), Request{
		Target: Target{
			Host:     "192.168.8.200; rm -rf /",
			Platform: "windows",
		},
	})
	if err == nil {
		t.Error("expected error on unsafe host")
	}

	// Test failure propagation
	_, err = svc.Execute(context.Background(), Request{
		Target: Target{
			Host:     "fail.local",
			Platform: "windows",
		},
	})
	if err == nil {
		t.Error("expected error on mock failure")
	}
}

func TestExecuteRejectsUnknownPlatformAndPartialDelays(t *testing.T) {
	svc := NewService(func(context.Context, Target, string) (string, error) {
		return "ok", nil
	})
	_, err := svc.Execute(context.Background(), Request{Target: Target{Host: "192.168.8.200"}})
	if err == nil {
		t.Fatal("expected unknown platform to fail")
	}
	_, err = svc.Execute(context.Background(), Request{
		Target: Target{Host: "192.168.8.200", Platform: "linux"},
		Delay:  90 * time.Second,
	})
	if err == nil {
		t.Fatal("expected sub-minute linux delay to fail")
	}
	_, err = svc.Execute(context.Background(), Request{
		Target: Target{Host: "192.168.8.200", Platform: "windows"},
		Delay:  500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected sub-second windows delay to fail")
	}
	_, err = svc.Execute(context.Background(), Request{
		Target: Target{Host: "192.168.8.200", Platform: "linux"},
		Force:  true,
	})
	if err == nil {
		t.Fatal("expected linux force to fail")
	}
}
