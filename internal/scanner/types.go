package scanner

// TargetType categorizes discovered entities
type TargetType string

const (
	TargetRouter   TargetType = "router"
	TargetZeroTier TargetType = "zerotier"
	TargetServer   TargetType = "server"
	TargetIosSim   TargetType = "ios-sim"
	TargetAndroid  TargetType = "android-adb"
	TargetSSHHost  TargetType = "ssh-host"
)

// DiscoveredTarget represents a discovered machine, simulator or device
type DiscoveredTarget struct {
	Type         TargetType `json:"type"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	IP           string     `json:"ip"`
	Port         int        `json:"port"`
	Status       string     `json:"status"` // "online", "offline", "booted", "connected"
	SSHReachable bool       `json:"sshReachable"`
	Details      string     `json:"details"`
	UDID         string     `json:"udid,omitempty"`
}
