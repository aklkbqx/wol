package tui

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
)

var (
	demoOnce  sync.Once
	demoBuilt *Demo
	demoErr   error
)

// testDemo is the phone-size walk, built once for all demo tests.
func testDemo(t *testing.T) *Demo {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the whole demo")
	}
	demoOnce.Do(func() {
		demoBuilt, demoErr = BuildDemo(context.Background(), DemoOptions{Width: 40, Height: 36, MaxStates: 8000, Version: "0.4.20"})
	})
	if demoErr != nil {
		t.Fatal(demoErr)
	}
	return demoBuilt
}

func demoScreen(d *Demo, state int) string {
	lines := make([]string, len(d.States[state].Rows))
	for i, r := range d.States[state].Rows {
		lines[i] = stripANSI(d.Rows[r])
	}
	return strings.Join(lines, "\n")
}

// demoWalk presses keys from the first screen, letting each screen that
// moves on by itself do so, and returns where it ends.
func demoWalk(t *testing.T, d *Demo, keys ...string) int {
	t.Helper()
	at := 0
	settle := func() {
		for steps := 0; d.States[at].Next != nil; steps++ {
			if steps > 50 {
				t.Fatalf("state %d never settles", at)
			}
			at = d.States[at].Next.State
		}
	}
	for _, key := range keys {
		next, ok := d.States[at].Keys[key]
		if !ok {
			t.Fatalf("key %q does nothing after %v on:\n%s", key, keys, demoScreen(d, at))
		}
		at = next
		settle()
	}
	return at
}

func TestDemoScreensFillTheTerminal(t *testing.T) {
	d := testDemo(t)
	if len(d.States) < 500 || len(d.States) >= 8000 {
		t.Fatalf("states = %d, want a full walk under the cap", len(d.States))
	}
	for i, s := range d.States {
		if len(s.Rows) != d.Height {
			t.Fatalf("state %d has %d rows, want %d", i, len(s.Rows), d.Height)
		}
	}
	for i, row := range d.Rows {
		if got := visibleWidth(row); got != d.Width {
			t.Fatalf("row %d is %d wide, want %d: %q", i, got, d.Width, stripANSI(row))
		}
	}
}

func TestDemoEveryScreenLeadsSomewhere(t *testing.T) {
	d := testDemo(t)
	for i, s := range d.States {
		if len(s.Keys) == 0 && s.Next == nil {
			t.Fatalf("state %d is a dead end:\n%s", i, demoScreen(d, i))
		}
		for key, next := range s.Keys {
			if next < 0 || next >= len(d.States) {
				t.Fatalf("state %d key %q points at %d", i, key, next)
			}
		}
	}
}

// The sample inventory uses documentation addresses only (RFC 5737) and
// locally administered MACs.
func TestDemoShowsOnlySampleAddresses(t *testing.T) {
	d := testDemo(t)
	ipv4 := regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	mac := regexp.MustCompile(`(?i)\b[0-9a-f]{2}(:[0-9a-f]{2}){5}\b`)
	for _, row := range d.Rows {
		text := stripANSI(row)
		for _, ip := range ipv4.FindAllString(text, -1) {
			if !strings.HasPrefix(ip, "198.51.100.") && !strings.HasPrefix(ip, "203.0.113.") {
				t.Fatalf("row shows %s: %q", ip, text)
			}
		}
		for _, m := range mac.FindAllString(text, -1) {
			if !strings.HasPrefix(m, "02:00:00:00:01:") {
				t.Fatalf("row shows %s: %q", m, text)
			}
		}
	}
}

func TestDemoWakeCountsDownAndAnswers(t *testing.T) {
	d := testDemo(t)
	at := 0
	next, ok := d.States[at].Keys["w"]
	if !ok {
		t.Fatal("w does nothing on the first screen")
	}
	ticks := 0
	for at = next; d.States[at].Next != nil; at = d.States[at].Next.State {
		if d.States[at].Next.After == int(demoTickAfter.Milliseconds()) {
			ticks++
		}
	}
	if ticks < demoAnswerAfter-1 {
		t.Fatalf("countdown ticks = %d, want about %d", ticks, demoAnswerAfter)
	}
	if got := demoScreen(d, at); !strings.Contains(got, "nas-vault") || !strings.Contains(got, "ONLINE after wake") {
		t.Fatalf("wake did not end online:\n%s", got)
	}
}

func TestDemoConnectStreams(t *testing.T) {
	d := testDemo(t)
	at := demoWalk(t, d, "down", "c")
	if got := demoScreen(d, at); !strings.Contains(got, "render-node · streaming") {
		t.Fatalf("connect did not stream:\n%s", got)
	}
	at = d.States[at].Keys["x"]
	if at == 0 {
		t.Fatal("x does not disconnect")
	}
}

func TestDemoBatchShowsResults(t *testing.T) {
	d := testDemo(t)
	at := demoWalk(t, d, "space", "down", "space", "W", "enter")
	got := demoScreen(d, at)
	if !strings.Contains(got, "nas-vault") || !strings.Contains(got, "render-node") || !strings.Contains(got, "sent") {
		t.Fatalf("batch results missing:\n%s", got)
	}
}

func TestDemoFilterTypesTheWord(t *testing.T) {
	d := testDemo(t)
	at := demoWalk(t, d, "/", "char", "char", "char", "enter")
	if got := demoScreen(d, at); !strings.Contains(got, "nas-vault") || strings.Contains(got, "render-node") {
		t.Fatalf("filter nas did not narrow the list:\n%s", got)
	}
}

func TestBuildDemoIsDeterministic(t *testing.T) {
	opts := DemoOptions{Width: 100, Height: 28, MaxStates: 300, Version: "0.4.20"}
	a, err := BuildDemo(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildDemo(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("two builds differ")
	}
}

func TestBuildDemoRejectsTinySizes(t *testing.T) {
	if _, err := BuildDemo(context.Background(), DemoOptions{Width: 10, Height: 5}); err == nil {
		t.Fatal("want an error below the smallest layout")
	}
}

func TestBuildDemoStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BuildDemo(ctx, DemoOptions{Width: 100, Height: 28}); err == nil {
		t.Fatal("want the context error")
	}
}
