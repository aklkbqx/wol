package doctor

import (
	"bytes"
	"debug/macho"
	"os"
	"runtime"
)

// localNetworkKey is the Info.plist key macOS requires before it lets a
// command-line tool probe hosts on the local network.
const localNetworkKey = "NSLocalNetworkUsageDescription"

// embeddedInfoPlist returns the __TEXT,__info_plist section of a Mach-O file.
// ok is false when the file is Mach-O but carries no embedded plist.
func embeddedInfoPlist(path string) (data []byte, ok bool, err error) {
	f, err := macho.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	sec := f.Section("__info_plist")
	if sec == nil || sec.Seg != "__TEXT" {
		return nil, false, nil
	}
	data, err = sec.Data()
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// packagingStatus reports whether a macOS executable carries the Local
// Network usage description that `make build` embeds. A plain `go install`
// or a source-build update skips it, and status checks then fail silently.
func packagingStatus(goos, exePath string) (status, details string, applies bool) {
	if goos != "darwin" {
		return "", "", false
	}
	data, ok, err := embeddedInfoPlist(exePath)
	switch {
	case err != nil:
		return "WARN", "cannot inspect executable: " + err.Error(), true
	case !ok || !bytes.Contains(data, []byte(localNetworkKey)):
		return "WARN", "missing Local Network usage description; reinstall with make install or a release archive", true
	}
	return "OK", "Local Network usage description embedded", true
}

func addPackaging(add func(cat, name, status, details string)) {
	exe, _ := os.Executable()
	if status, details, applies := packagingStatus(runtime.GOOS, exe); applies {
		add("Packaging", "macOS Local Network", status, details)
	}
}
