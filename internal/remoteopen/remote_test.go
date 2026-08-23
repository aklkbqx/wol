package remoteopen

import (
	"context"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"https://wol.example.test/remote/device-1",
		"http://192.168.50.5/remote/device-1",
	} {
		if got, err := Validate(target); err != nil || got != target {
			t.Fatalf("Validate(%q) = %q, %v", target, got, err)
		}
	}
	for _, target := range []string{"", "example.test", "file:///tmp/a", "rdp://192.168.50.200", "vnc://192.168.50.5", "https://user:secret@example.test", "https://example.test/?code=value", "https://example.test/#session", "https://example.test/\nnext"} {
		if _, err := Validate(target); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", target)
		}
	}
}

func TestCommandDoesNotUseShell(t *testing.T) {
	t.Parallel()
	cmd, err := Command(context.Background(), "https://example.test/remote/a%20b")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if strings.Contains(joined, "sh -c") {
		t.Fatalf("remote opener invoked a shell: %q", joined)
	}
}
