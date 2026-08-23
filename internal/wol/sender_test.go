package wol

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSendWritesMagicPacketToLoopback(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	mac, err := ParseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Send(context.Background(), SendRequest{MAC: mac, Destination: net.IPv4(127, 0, 0, 1), Port: listener.LocalAddr().(*net.UDPAddr).Port, Repeat: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Packets != 1 || result.Bytes != 102 {
		t.Fatalf("unexpected send result: %+v", result)
	}
	if err := listener.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 256)
	count, _, err := listener.ReadFromUDP(packet)
	if err != nil {
		t.Fatal(err)
	}
	if count != 102 {
		t.Fatalf("received packet length = %d, want 102", count)
	}
}
