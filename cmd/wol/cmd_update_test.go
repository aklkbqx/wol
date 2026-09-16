package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRunUpdateInvalidFlag(t *testing.T) {
	code := runUpdate([]string{"--nonexistent-flag"})
	if code != 2 {
		t.Errorf("expected exit code 2 for invalid flag, got %d", code)
	}
}

func TestRunUpdateExtraArgs(t *testing.T) {
	code := runUpdate([]string{"extra-arg"})
	if code != 2 {
		t.Errorf("expected exit code 2 for unexpected positional argument, got %d", code)
	}
}

func TestRunUpdateCheckUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v` + appVersion + `",
			"name": "WOL v` + appVersion + `",
			"html_url": "https://github.com/aklkbqx/wol/releases/tag/v` + appVersion + `",
			"assets": []
		}`))
	}))
	defer server.Close()

	// Redirect stdout to capture output
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	code := runUpdate([]string{"--check", "--repo", "aklkbqx/wol"})

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if code != 0 {
		t.Errorf("expected exit code 0, got %d, output: %s", code, output)
	}
	if !strings.Contains(output, "up to date") {
		t.Errorf("expected output to mention 'up to date', got: %s", output)
	}
}
