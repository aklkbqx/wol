package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadEnvFileAndProcessOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	contents := "# comment\nPLAIN=value\nQUOTED=\"hello world\"\nexport SINGLE='one two'\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAIN", "from-process")

	got, err := LoadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"QUOTED=hello world", "SINGLE=one two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadEnvFile() = %#v, want %#v", got, want)
	}
	if value := EnvValue("PLAIN", got, "fallback"); value != "from-process" {
		t.Fatalf("EnvValue process override = %q", value)
	}
	if value := EnvValue("QUOTED", got, "fallback"); value != "hello world" {
		t.Fatalf("EnvValue file default = %q", value)
	}
}

func TestLoadEnvFileRejectsInvalidAssignment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("NOT VALID\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEnvFile(path); err == nil {
		t.Fatal("LoadEnvFile() expected an error")
	}
}
