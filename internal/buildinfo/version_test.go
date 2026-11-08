package buildinfo

import (
	"regexp"
	"strings"
	"testing"
)

var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

func TestVersionAndCreditMetadata(t *testing.T) {
	if strings.TrimSpace(Version) == "" {
		t.Fatal("Version must not be empty")
	}
	if Version != strings.TrimSpace(Version) {
		t.Fatalf("Version %q contains leading or trailing whitespace", Version)
	}
	if strings.HasPrefix(Version, "v") || strings.HasPrefix(Version, "V") {
		t.Fatalf("Version %q must not carry a leading 'v' prefix", Version)
	}
	if !semverPattern.MatchString(Version) {
		t.Fatalf("Version %q does not match expected semantic version format", Version)
	}
	if strings.TrimSpace(Credit) == "" {
		t.Fatal("Credit must not be empty")
	}
	if Credit != "aklkbqx" {
		t.Fatalf("Credit = %q, want %q", Credit, "aklkbqx")
	}
	if got := IsDevelopment(); got != strings.HasSuffix(Version, "-dev") {
		t.Fatalf("IsDevelopment() = %v for Version=%q", got, Version)
	}
}

func TestIsDevVersionGuard(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"0.4.19-dev", true},
		{"0.4.19-DEV", true},
		{" 0.4.19-dev ", true},
		{"0.4.18", false},
		{"0.4.19-rc.1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsDevVersion(tc.version); got != tc.want {
			t.Fatalf("IsDevVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
