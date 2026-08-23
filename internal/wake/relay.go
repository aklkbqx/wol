package wake

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

// SendEtherwake invokes the router's etherwake binary through a non-
// interactive SSH connection. All dynamic values are validated before they
// are placed in the remote command.
func SendEtherwake(ctx context.Context, relay store.WakeRelay, mac net.HardwareAddr) (RelayResult, error) {
	if strings.TrimSpace(relay.Address) == "" {
		return RelayResult{}, fmt.Errorf("relay %q has no SSH address", relay.Name)
	}
	if strings.TrimSpace(relay.Transport) != "" && !strings.EqualFold(strings.TrimSpace(relay.Transport), "ssh_etherwake") {
		return RelayResult{}, fmt.Errorf("relay transport %q is not supported", relay.Transport)
	}
	if len(mac) != 6 {
		return RelayResult{}, errorsForRelay("invalid MAC address")
	}
	if !safeSSHToken(relay.Address) || (relay.SSHUser != "" && !safeSSHToken(relay.SSHUser)) {
		return RelayResult{}, errorsForRelay("relay SSH address contains unsafe characters")
	}
	interfaceName := strings.TrimSpace(relay.Interface)
	if interfaceName == "" {
		interfaceName = "br-lan"
	}
	if !safeSSHToken(interfaceName) {
		return RelayResult{}, errorsForRelay("relay interface contains unsafe characters")
	}

	port := relay.Port
	if port < 0 || port > 65535 {
		return RelayResult{}, errorsForRelay("relay SSH port is invalid")
	}
	if port <= 0 {
		port = 22
	}
	target := strings.TrimSpace(relay.Address)
	if relay.SSHUser != "" {
		target = strings.TrimSpace(relay.SSHUser) + "@" + target
	}
	remote := fmt.Sprintf("/usr/bin/etherwake -b -i %s %s", interfaceName, mac.String())
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=2"}
	if port != 22 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	args = append(args, target, remote)
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(callCtx, "ssh", args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return RelayResult{}, fmt.Errorf("ssh etherwake failed: %s", message)
	}
	return RelayResult{Packets: 1, Detail: strings.TrimSpace(string(output))}, nil
}

func safeSSHToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r <= ' ' || strings.ContainsRune("'\";&|$`\\<>\n\r", r) {
			return false
		}
	}
	return true
}

type relayError string

func (e relayError) Error() string { return string(e) }

func errorsForRelay(message string) error { return relayError(message) }
