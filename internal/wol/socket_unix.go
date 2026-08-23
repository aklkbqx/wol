//go:build !windows

package wol

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func listenUDP(ctx context.Context, localIP net.IP) (*net.UDPConn, error) {
	config := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var controlErr error
		if err := raw.Control(func(fd uintptr) {
			controlErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
		}); err != nil {
			return err
		}
		return controlErr
	}}
	address := "0.0.0.0:0"
	if localIP != nil && !localIP.IsUnspecified() {
		address = (&net.UDPAddr{IP: localIP, Port: 0}).String()
	}
	packetConn, err := config.ListenPacket(ctx, "udp4", address)
	if err != nil {
		return nil, err
	}
	return packetConn.(*net.UDPConn), nil
}
