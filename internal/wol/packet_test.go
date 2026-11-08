package wol

import (
	"bytes"
	"net"
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

func TestParseMACPadsShortARPOctets(t *testing.T) {
	mac, err := ParseMAC("30:56:f:7f:c4:5d")
	if err != nil {
		t.Fatal(err)
	}
	if mac.String() != "30:56:0f:7f:c4:5d" {
		t.Fatalf("short ARP octet: %s", mac)
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
	for _, input := range []string{
		"",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"01:00:00:00:00:00",
		"not-a-mac",
		":02:00:00:00:0001",
		"02:00-00:00:00:01",
		"0200.00.000001",
	} {
		if _, err := ParseMAC(input); err == nil {
			t.Fatalf("ParseMAC(%q) succeeded", input)
		}
	}
}

func FuzzParseMAC(f *testing.F) {
	for _, seed := range []string{
		"aa:bb:cc:dd:ee:ff",
		"AA-BB-CC-DD-EE-FF",
		"aabb.ccdd.eeff",
		"aabbccddeeff",
		"30:56:f:7f:c4:5d",
		"",
		"00:00:00:00:00:00",
		"ff:ff:ff:ff:ff:ff",
		"01:02:03:04:05:06",
		":02:00:00:00:0001",
		"02:00-00:00:00:01",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		mac, err := ParseMAC(input)
		if err != nil {
			return
		}
		if len(mac) != 6 {
			t.Fatalf("ParseMAC(%q) returned len=%d, want 6", input, len(mac))
		}
		if mac[0]&1 == 1 {
			t.Fatalf("ParseMAC(%q) accepted multicast MAC %s", input, mac)
		}
		if isAllZeroMAC(mac) {
			t.Fatalf("ParseMAC(%q) accepted all-zero MAC", input)
		}
		roundTrip, err := ParseMAC(mac.String())
		if err != nil || !bytes.Equal(mac, roundTrip) {
			t.Fatalf("ParseMAC round-trip failed for %q -> %s: %v", input, mac, err)
		}
		packet, err := BuildMagicPacket(mac)
		if err != nil {
			t.Fatalf("BuildMagicPacket(%s) failed after ParseMAC(%q): %v", mac, input, err)
		}
		if len(packet) != 102 {
			t.Fatalf("BuildMagicPacket(%s) len = %d, want 102", mac, len(packet))
		}
	})
}

func FuzzBuildMagicPacket(f *testing.F) {
	f.Add([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	f.Add([]byte{0x02, 0x03})
	f.Fuzz(func(t *testing.T, raw []byte) {
		mac := net.HardwareAddr(raw)
		packet, err := BuildMagicPacket(mac)
		if len(raw) != 6 || isAllZeroMAC(mac) || raw[0]&1 == 1 {
			if err == nil {
				t.Fatalf("BuildMagicPacket(%x) succeeded on invalid MAC", raw)
			}
			return
		}
		if err != nil {
			t.Fatalf("BuildMagicPacket(%x) unexpected error: %v", raw, err)
		}
		if len(packet) != 102 {
			t.Fatalf("len(packet) = %d, want 102", len(packet))
		}
		for i := 0; i < 6; i++ {
			if packet[i] != 0xff {
				t.Fatalf("sync byte %d = %#x, want 0xff", i, packet[i])
			}
		}
		for offset := 6; offset < 102; offset += 6 {
			if !bytes.Equal(packet[offset:offset+6], raw) {
				t.Fatalf("payload mismatch at offset %d", offset)
			}
		}
	})
}
