package remoteopen

import (
	"context"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"http://127.0.0.1:49152/s/device-1",
		"http://localhost:49152/s/device-1",
	} {
		if got, err := Validate(target); err != nil || got != target {
			t.Fatalf("Validate(%q) = %q, %v", target, got, err)
		}
	}
	for _, target := range []string{"", "example.test", "file:///tmp/a", "https://127.0.0.1/session", "http://192.168.50.5/session", "http://user:secret@127.0.0.1/session", "http://127.0.0.1/?code=value", "http://127.0.0.1/#session", "http://127.0.0.1/\nnext"} {
		if _, err := Validate(target); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", target)
		}
	}
}

func TestCommandDoesNotUseShell(t *testing.T) {
	t.Parallel()
	cmd, err := Command(context.Background(), "http://127.0.0.1:49152/s/a%20b")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if strings.Contains(joined, "sh -c") {
		t.Fatalf("remote opener invoked a shell: %q", joined)
	}
}
