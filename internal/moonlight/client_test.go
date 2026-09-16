package moonlight

import (
	"errors"
	"testing"
)

func TestBuildStreamArgs(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	args := client.BuildStreamArgs("192.168.8.200", "Desktop", 120, "2560x1440", 50000)
	expected := []string{"stream", "192.168.8.200", "Desktop", "--fps", "120", "--resolution", "2560x1440", "--bitrate", "50000"}
	assertArgs(t, args, expected)
}

func TestBuildStreamArgsDefaults(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	args := client.BuildStreamArgs("192.168.8.200", "", 0, "", 0)
	assertArgs(t, args, []string{"stream", "192.168.8.200", "Desktop"})
}

func TestBuildStreamArgsFlatpakPrefix(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/bin/flatpak", FlatpakApp: "com.moonlight_stream.Moonlight"}
	args := client.BuildStreamArgs("10.0.0.8", "Desktop", 60, "1920x1080", 0)
	assertArgs(t, args, []string{"run", "com.moonlight_stream.Moonlight", "stream", "10.0.0.8", "Desktop", "--fps", "60", "--resolution", "1920x1080"})
}

func TestBuildPairArgs(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	assertArgs(t, client.BuildPairArgs("192.168.8.200", ""), []string{"pair", "192.168.8.200"})
	assertArgs(t, client.BuildPairArgs("192.168.8.200", "1234"), []string{"pair", "192.168.8.200", "--pin", "1234"})
}

func TestDetectWithLookPath(t *testing.T) {
	origLookPath := LookPathFunc
	defer func() { LookPathFunc = origLookPath }()

	LookPathFunc = func(file string) (string, error) {
		if file == "moonlight" {
			return "/usr/bin/moonlight", nil
		}
		return "", errors.New("not found")
	}

	client, err := Detect()
	if err != nil {
		t.Fatalf("expected client found, got err: %v", err)
	}
	if client.ExecutablePath != "/usr/bin/moonlight" {
		t.Fatalf("unexpected executable path: %s", client.ExecutablePath)
	}
}

func TestDetectNotFound(t *testing.T) {
	origLookPath := LookPathFunc
	origFileExists := FileExistsFunc
	defer func() {
		LookPathFunc = origLookPath
		FileExistsFunc = origFileExists
	}()

	LookPathFunc = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	FileExistsFunc = func(path string) bool {
		return false
	}

	client, err := Detect()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got client: %+v, err: %v", client, err)
	}
}

func TestDetectIgnoresBareFlatpak(t *testing.T) {
	origLookPath := LookPathFunc
	origFileExists := FileExistsFunc
	defer func() {
		LookPathFunc = origLookPath
		FileExistsFunc = origFileExists
	}()
	LookPathFunc = func(file string) (string, error) {
		if file == "flatpak" {
			return "/usr/bin/flatpak", nil
		}
		return "", errors.New("not found")
	}
	FileExistsFunc = func(path string) bool { return false }
	if _, err := Detect(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("flatpak without moonlight app should be missing: %v", err)
	}
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args length mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}
