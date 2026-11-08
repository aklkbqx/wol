package wol

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

var ErrInvalidMAC = errors.New("invalid MAC address")

type SendRequest struct {
	MAC         net.HardwareAddr
	Destination net.IP
	Port        int
	Interface   string
	Repeat      int
	Interval    time.Duration
}

type SendResult struct {
	Destination string `json:"destination"`
	Port        int    `json:"port"`
	Packets     int    `json:"packets"`
	Bytes       int    `json:"bytes"`
	Warning     string `json:"warning,omitempty"`
}

func isAllZeroMAC(mac net.HardwareAddr) bool {
	return len(mac) == 6 && (mac[0]|mac[1]|mac[2]|mac[3]|mac[4]|mac[5]) == 0
}

func ParseMAC(value string) (net.HardwareAddr, error) {
	clean := macDigits(value)
	if len(clean) != 12 {
		return nil, ErrInvalidMAC
	}
	decoded, err := hex.DecodeString(clean)
	if err != nil || len(decoded) != 6 {
		return nil, ErrInvalidMAC
	}
	mac := net.HardwareAddr(decoded)
	if isAllZeroMAC(mac) || mac[0]&1 == 1 {
		return nil, ErrInvalidMAC
	}
	return mac, nil
}

func macDigits(value string) string {
	trimmed := strings.TrimSpace(value)
	hasColon := strings.Contains(trimmed, ":")
	hasHyphen := strings.Contains(trimmed, "-")
	hasDot := strings.Contains(trimmed, ".")
	sepCount := 0
	if hasColon {
		sepCount++
	}
	if hasHyphen {
		sepCount++
	}
	if hasDot {
		sepCount++
	}
	if sepCount > 1 {
		return ""
	}
	if hasColon || hasHyphen {
		sep := ":"
		if hasHyphen {
			sep = "-"
		}
		parts := strings.Split(trimmed, sep)
		if len(parts) != 6 {
			return ""
		}
		var b strings.Builder
		b.Grow(12)
		for _, part := range parts {
			if len(part) < 1 || len(part) > 2 {
				return ""
			}
			if len(part) == 1 {
				b.WriteByte('0')
			}
			b.WriteString(part)
		}
		return b.String()
	}
	if hasDot {
		parts := strings.Split(trimmed, ".")
		if len(parts) != 3 {
			return ""
		}
		var b strings.Builder
		b.Grow(12)
		for _, part := range parts {
			if len(part) != 4 {
				return ""
			}
			b.WriteString(part)
		}
		return b.String()
	}
	return trimmed
}

func BuildMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 || isAllZeroMAC(mac) || mac[0]&1 == 1 {
		return nil, ErrInvalidMAC
	}
	packet := make([]byte, 6+16*6)
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for offset := 6; offset < len(packet); offset += 6 {
		copy(packet[offset:offset+6], mac)
	}
	return packet, nil
}

func Send(ctx context.Context, request SendRequest) (SendResult, error) {
	if request.Destination == nil || request.Destination.To4() == nil {
		return SendResult{}, errors.New("destination must be an IPv4 address")
	}
	if request.Port < 1 || request.Port > 65535 {
		return SendResult{}, errors.New("port must be between 1 and 65535")
	}
	if request.Repeat < 1 {
		request.Repeat = 3
	}
	if request.Interval < 0 {
		return SendResult{}, errors.New("interval cannot be negative")
	}
	packet, err := BuildMagicPacket(request.MAC)
	if err != nil {
		return SendResult{}, err
	}
	var ifaceWarning string
	localAddress, err := localIPv4(request.Interface)
	if err != nil {
		localAddress = net.IPv4zero
		if strings.TrimSpace(request.Interface) != "" {
			ifaceWarning = fmt.Sprintf("interface %q unavailable; sending from 0.0.0.0", request.Interface)
		}
	}
	conn, err := listenUDP(ctx, localAddress)
	if err != nil {
		return SendResult{}, fmt.Errorf("open UDP socket: %w", err)
	}
	defer conn.Close()

	destination := &net.UDPAddr{IP: request.Destination.To4(), Port: request.Port}
	result := SendResult{Destination: destination.IP.String(), Port: destination.Port, Warning: ifaceWarning}
	for i := 0; i < request.Repeat; i++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		written, writeErr := conn.WriteToUDP(packet, destination)
		if writeErr != nil {
			return result, fmt.Errorf("send magic packet: %w", writeErr)
		}
		result.Packets++
		result.Bytes += written
		if i < request.Repeat-1 && request.Interval > 0 {
			timer := time.NewTimer(request.Interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return result, nil
}

func localIPv4(interfaceName string) (net.IP, error) {
	if interfaceName == "" {
		return net.IPv4zero, nil
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("interface %q not found: %w", interfaceName, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("read interface %q: %w", interfaceName, err)
	}
	for _, address := range addresses {
		var ip net.IP
		switch value := address.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4, nil
		}
	}
	return nil, fmt.Errorf("interface %q has no IPv4 address", interfaceName)
}
