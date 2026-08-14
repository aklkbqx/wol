package wol

import (
	"bytes"
	"testing"
)

func TestParseMACNormalizesSeparators(t *testing.T) {
	for _, input := range []string{"aa:bb:cc:dd:ee:ff", "AA-BB-CC-DD-EE-FF", "aabb.ccdd.eeff", "aabbccddeeff"} {
		mac, err := ParseMAC(input)
		if err != nil {
			t.Fatalf("ParseMAC(%q): %v", input, err)
		}
		if mac.String() != "aa:bb:cc:dd:ee:ff" {
			t.Fatalf("unexpected MAC: %s", mac)
		}
	}
}

func TestBuildMagicPacket(t *testing.T) {
	mac, err := ParseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	packet, err := BuildMagicPacket(mac)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 102 {
		t.Fatalf("packet length = %d, want 102", len(packet))
	}
	if !bytes.Equal(packet[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatalf("packet prefix is not six FF bytes")
	}
	for offset := 6; offset < len(packet); offset += 6 {
		if !bytes.Equal(packet[offset:offset+6], mac) {
			t.Fatalf("MAC repetition at offset %d is incorrect", offset)
		}
	}
}

func TestParseMACRejectsInvalidAddresses(t *testing.T) {
	for _, input := range []string{"", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "01:00:00:00:00:00", "not-a-mac"} {
		if _, err := ParseMAC(input); err == nil {
			t.Fatalf("ParseMAC(%q) succeeded", input)
		}
	}
}
