package presence

import "testing"

func TestParseNeighborOutput(t *testing.T) {
	darwinLive := "" +
		"? (192.168.8.200) at 30:56:f:7f:c4:5d on en0 ifscope [ethernet]\n" +
		"? (192.168.8.200) at 30:56:f:7f:c4:5d on en1 ifscope [ethernet]\n" +
		"? (192.168.8.200) at (incomplete) on bridge100 ifscope [bridge]\n" +
		"? (192.168.8.200) at da:9b:d0:54:e2:80 on bridge101 ifscope [bridge]\n"

	if !parseNeighborOutput("darwin", darwinLive) {
		t.Fatal("expected complete en0 ARP to count as a neighbor")
	}
	listed := parseNeighborTable("darwin", darwinLive)
	if len(listed) != 1 || listed[0].IP != "192.168.8.200" || listed[0].MAC != "30:56:0f:7f:c4:5d" {
		t.Fatalf("lan table = %+v", listed)
	}

	darwinBridgeOnly := "" +
		"? (192.168.8.200) at (incomplete) on bridge100 ifscope [bridge]\n" +
		"? (192.168.8.200) at da:9b:d0:54:e2:80 on bridge101 ifscope [bridge]\n"
	if parseNeighborOutput("darwin", darwinBridgeOnly) {
		t.Fatal("bridge/virtual ARP must not count as a neighbor")
	}

	darwinIncomplete := "? (192.168.8.200) at (incomplete) on en0 ifscope [ethernet]\n"
	if parseNeighborOutput("darwin", darwinIncomplete) {
		t.Fatal("incomplete ARP must not count as a neighbor")
	}

	linuxReachable := "192.168.8.200 dev eth0 lladdr 30:56:0f:7f:c4:5d REACHABLE\n"
	if !parseNeighborOutput("linux", linuxReachable) {
		t.Fatal("expected linux REACHABLE neighbor")
	}

	linuxFailed := "192.168.8.200 dev eth0 FAILED\n"
	if parseNeighborOutput("linux", linuxFailed) {
		t.Fatal("linux FAILED neighbor must not count")
	}
	if parseNeighborOutput("linux", "192.168.8.200 dev eth0 lladdr 30:56:0f:7f:c4:5d FAILED\n") {
		t.Fatal("FAILED neighbor with a cached MAC must not count")
	}

	linuxDocker := "192.168.8.200 dev docker0 lladdr 30:56:0f:7f:c4:5d STALE\n"
	if parseNeighborOutput("linux", linuxDocker) {
		t.Fatal("docker bridge neighbor must not count")
	}

	windowsDyn := "" +
		"Interface: 192.168.8.20 --- 0x7\n" +
		"  Internet Address      Physical Address      Type\n" +
		"  192.168.8.200         30-56-0f-7f-c4-5d     dynamic\n"
	if !parseNeighborOutput("windows", windowsDyn) {
		t.Fatal("expected windows dynamic ARP to count")
	}
}

func TestIsPhysicalIface(t *testing.T) {
	physical := []string{"en0", "en1", "eth0", "wlan0", "enp3s0"}
	for _, name := range physical {
		if !isPhysicalIface(name) {
			t.Errorf("%s should be physical", name)
		}
	}
	virtual := []string{"lo0", "bridge100", "bridge101", "feth4646", "utun2", "docker0", "veth0", "awdl0"}
	for _, name := range virtual {
		if isPhysicalIface(name) {
			t.Errorf("%s should be virtual", name)
		}
	}
}

func TestIsIgnoredMAC(t *testing.T) {
	if !isIgnoredMAC("00:00:00:00:00:00") || !isIgnoredMAC("ff:ff:ff:ff:ff:ff") {
		t.Fatal("zero and broadcast MACs should be ignored")
	}
	if isIgnoredMAC("30:56:f:7f:c4:5d") {
		t.Fatal("real MAC should not be ignored")
	}
}
