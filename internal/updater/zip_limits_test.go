package updater

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"
)

func boundedZipFixture(t testing.TB) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("wol")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("fixture")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestZipPreflightFormats(t *testing.T) {
	content := boundedZipFixture(t)
	end := len(content) - 22
	zip64 := make([]byte, 56)
	binary.LittleEndian.PutUint32(zip64, 0x06064b50)
	binary.LittleEndian.PutUint64(zip64[4:], 44)
	binary.LittleEndian.PutUint64(zip64[24:], 1)
	binary.LittleEndian.PutUint64(zip64[32:], 1)
	binary.LittleEndian.PutUint64(zip64[40:], uint64(binary.LittleEndian.Uint32(content[end+12:])))
	binary.LittleEndian.PutUint64(zip64[48:], uint64(binary.LittleEndian.Uint32(content[end+16:])))
	locator := make([]byte, 20)
	binary.LittleEndian.PutUint32(locator, 0x07064b50)
	binary.LittleEndian.PutUint64(locator[8:], uint64(end))
	binary.LittleEndian.PutUint32(locator[16:], 1)
	last := append([]byte(nil), content[end:]...)
	binary.LittleEndian.PutUint16(last[8:], 0xffff)
	binary.LittleEndian.PutUint16(last[10:], 0xffff)
	binary.LittleEndian.PutUint32(last[12:], 0xffffffff)
	binary.LittleEndian.PutUint32(last[16:], 0xffffffff)
	wide := append(append(append(append([]byte(nil), content[:end]...), zip64...), locator...), last...)
	for name, data := range map[string][]byte{"normal": content, "prefixed": append([]byte("prefix"), content...), "zip64": wide} {
		t.Run(name, func(t *testing.T) {
			if err := preflightZipDirectory(data, 1); err != nil {
				t.Fatal(err)
			}
			got, err := ExtractBinary(data, "wol.zip")
			if err != nil || string(got) != "fixture" {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
	for name, mutate := range map[string]func([]byte){
		"invalid comment": func(b []byte) { binary.LittleEndian.PutUint16(b[end+20:], 1) },
		"invalid size":    func(b []byte) { binary.LittleEndian.PutUint32(b[end+12:], 0xfffffffe) },
		"invalid offset":  func(b []byte) { binary.LittleEndian.PutUint32(b[end+16:], 0xfffffffe) },
	} {
		t.Run(name, func(t *testing.T) {
			data := append([]byte(nil), content...)
			mutate(data)
			if err := preflightZipDirectory(data, 1); err == nil {
				t.Fatal("malformed directory accepted")
			}
		})
	}
}

func FuzzZipDirectoryPreflight(f *testing.F) {
	f.Add(boundedZipFixture(f))
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte("PK"), 100))
	f.Fuzz(func(t *testing.T, content []byte) {
		_ = preflightZipDirectory(content, 4)
	})
}
