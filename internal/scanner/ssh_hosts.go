package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// GetKnownSSHHosts parses ~/.ssh/config and returns unique host alias names
func GetKnownSSHHosts() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{"router", "private", "private2", "macbook"}
	}

	configPath := filepath.Join(home, ".ssh", "config")
	f, err := os.Open(configPath)
	if err != nil {
		return []string{"router", "private", "private2", "macbook"}
	}
	defer f.Close()

	hostSet := make(map[string]struct{})
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "host ") {
			parts := strings.Fields(line)
			for _, alias := range parts[1:] {
				if !strings.Contains(alias, "*") && alias != "github.com" {
					hostSet[alias] = struct{}{}
				}
			}
		}
	}

	var list []string
	for h := range hostSet {
		list = append(list, h)
	}
	return list
}
