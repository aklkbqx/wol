package netutil

import "testing"

func TestBroadcastMasks(t *testing.T) {
	for cidr, want := range map[string]string{"10.2.3.4/16": "10.2.255.255", "192.0.2.12/23": "192.0.3.255", "192.0.2.12/24": "192.0.2.255", "192.0.2.12/30": "192.0.2.15"} {
		got, err := Broadcast(cidr)
		if err != nil || got != want {
			t.Fatalf("%s: %s %v", cidr, got, err)
		}
	}
	for _, cidr := range []string{"bad", "::1/64", "192.0.2.1/31", "192.0.2.1/32"} {
		if _, err := Broadcast(cidr); err == nil {
			t.Fatalf("accepted %s", cidr)
		}
	}
}
func TestUnknownInterfaceDoesNotGuess(t *testing.T) {
	if got := LocalBroadcast("192.0.2.1", "wol-no-such-interface"); got != "" {
		t.Fatal(got)
	}
}
