package scanner

import (
	"testing"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
)

func TestClassifyLANAndBroadcast(t *testing.T) {
	devices := []store.Device{
		{ID: "d1", Name: "windows", MACAddress: "30:56:0f:7f:c4:5d", IPAddress: "192.168.8.10"},
	}
	neighbors := []presence.Neighbor{
		{IP: "192.168.8.200", MAC: "30:56:0f:7f:c4:5d", Iface: "en0"},
		{IP: "192.168.8.40", MAC: "aa:bb:cc:dd:ee:01", Iface: "en0"},
	}
	known, moved, unknown := ClassifyLAN(devices, neighbors)
	if len(known) != 0 || len(moved) != 1 || len(unknown) != 1 {
		t.Fatalf("known=%d moved=%d unknown=%d", len(known), len(moved), len(unknown))
	}
	if moved[0].Neighbor.IP != "192.168.8.200" {
		t.Fatalf("moved IP = %s", moved[0].Neighbor.IP)
	}

	if UniqueDeviceName(devices, "windows") == "windows" {
		t.Fatal("expected unique name to avoid colliding with windows")
	}
}
