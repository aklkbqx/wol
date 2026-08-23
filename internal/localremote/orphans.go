package localremote

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// cleanupOrphans removes only resources owned by a WOL process that no longer
// exists. Concurrent sessions owned by live processes are left untouched.
func cleanupOrphans(ctx context.Context, runner Runner) error {
	containers, err := runner.Run(ctx, Command{Name: "docker", Args: []string{"ps", "--all", "--quiet", "--filter", "label=" + ownershipLabel + "=true"}})
	if err != nil {
		return commandError("list local remote containers", containers, err)
	}
	for _, id := range strings.Fields(containers.Stdout) {
		owner, inspectErr := runner.Run(ctx, Command{Name: "docker", Args: []string{"inspect", "--format", "{{ index .Config.Labels \"" + ownerPIDLabel + "\" }}", id}})
		if inspectErr != nil {
			continue
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(owner.Stdout))
		if parseErr != nil || pid < 1 || processAlive(pid) {
			continue
		}
		removed, removeErr := runner.Run(ctx, Command{Name: "docker", Args: []string{"rm", "--force", id}})
		if removeErr != nil {
			return commandError("remove orphaned local remote container", removed, removeErr)
		}
	}

	networks, err := runner.Run(ctx, Command{Name: "docker", Args: []string{"network", "ls", "--quiet", "--filter", "label=" + ownershipLabel + "=true"}})
	if err != nil {
		return commandError("list local remote networks", networks, err)
	}
	for _, id := range strings.Fields(networks.Stdout) {
		owner, inspectErr := runner.Run(ctx, Command{Name: "docker", Args: []string{"network", "inspect", "--format", "{{ index .Labels \"" + ownerPIDLabel + "\" }}", id}})
		if inspectErr != nil {
			continue
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(owner.Stdout))
		if parseErr != nil || pid < 1 || processAlive(pid) {
			continue
		}
		removed, removeErr := runner.Run(ctx, Command{Name: "docker", Args: []string{"network", "rm", id}})
		if removeErr != nil && !strings.Contains(strings.ToLower(removed.Stderr), "not found") {
			return fmt.Errorf("remove orphaned local remote network: %w", commandError("docker network rm", removed, removeErr))
		}
	}
	return nil
}
