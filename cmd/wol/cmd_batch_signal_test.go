//go:build !windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/store"
)

func TestBatchSignalHelper(t *testing.T) {
	if os.Getenv("WOL_TEST_INTERRUPT_HELPER") != "yes" {
		return
	}
	os.Exit(runBatch([]string{"check", "--db", os.Getenv("WOL_TEST_INTERRUPT_DB"), "--all", "--json"}))
}

func TestBatchSignalsStopCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wol.db")
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	const total = 2000
	for i := 0; i < total; i++ {
		// Missing IPs return unknown without any network operations. Enough output
		// fills the pipe so the child stays in its batch until the signal arrives.
		_, err = repo.CreateDevice(t.Context(), store.Device{Name: fmt.Sprintf("pc-%04d", i), MACAddress: fmt.Sprintf("02:00:00:00:%02x:%02x", i/256, i%256), Enabled: true})
		if err != nil {
			repo.Close()
			t.Fatal(err)
		}
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBatchSignalHelper$")
			cmd.Env = append(os.Environ(), "WOL_TEST_INTERRUPT_HELPER=yes", "WOL_TEST_INTERRUPT_DB="+path)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() {
				t.Fatalf("no first result: %s", stderr.String())
			}
			first := append([]byte(nil), scanner.Bytes()...)
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			var result fleet.Result
			if err := json.Unmarshal(first, &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != "unknown" {
				t.Fatalf("first result=%s", result.Status)
			}
			count := 1
			seen := map[string]bool{result.DeviceID: true}
			for scanner.Scan() {
				if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if seen[result.DeviceID] {
					t.Fatal("duplicate result")
				}
				seen[result.DeviceID] = true
				count++
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 130 || count != total {
				t.Fatalf("signal=%v exit=%v results=%d stderr=%s", sig, err, count, stderr.String())
			}
			if ctx.Err() != nil {
				t.Fatal("batch did not stop before deadline")
			}
		})
	}
}
