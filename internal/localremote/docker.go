package localremote

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	GuacamoleImage = "guacamole/guacamole@sha256:f344085e618bb05e22b964b0208dbd06d3468275bac70206f93805245e067b40"
	GuacdImage     = "guacamole/guacd@sha256:8974eaa9ba32f713daf311e7cc8cd7e4cdfba1edea39eed75524e78ef4b08f4f"
	ownershipLabel = "dev.aklkbqx.wol.local-remote"
	ownerPIDLabel  = "dev.aklkbqx.wol.owner-pid"
)

// ImageLabel returns a concise human-readable label without weakening the
// immutable digest used by Docker commands.
func ImageLabel(image string) string {
	name, _, _ := strings.Cut(image, "@")
	return name + ":1.6.0 (digest pinned)"
}

// DoctorReport describes whether the optional browser-remote runtime is ready.
type DoctorReport struct {
	DockerCLI    bool
	DockerDaemon bool
	Images       map[string]bool
	Problems     []string
}

func (r DoctorReport) Ready() bool {
	return r.DockerCLI && r.DockerDaemon && r.Images[GuacamoleImage] && r.Images[GuacdImage]
}

// Doctor checks Docker and the pinned images without changing the system.
func Doctor(ctx context.Context, runners ...Runner) (DoctorReport, error) {
	var injected Runner
	if len(runners) > 0 {
		injected = runners[0]
	}
	runner := runnerOrDefault(injected)
	report := DoctorReport{Images: map[string]bool{GuacamoleImage: false, GuacdImage: false}}
	if _, err := runner.LookPath("docker"); err != nil {
		report.Problems = append(report.Problems, "Docker CLI is not installed or not in PATH")
		return report, nil
	}
	report.DockerCLI = true
	result, err := runner.Run(ctx, Command{Name: "docker", Args: []string{"version", "--format", "{{.Server.Version}}"}})
	if err != nil {
		report.Problems = append(report.Problems, "Docker daemon is unavailable: "+commandError("docker version", result, err).Error())
		return report, nil
	}
	report.DockerDaemon = true
	for _, image := range []string{GuacamoleImage, GuacdImage} {
		result, err = runner.Run(ctx, Command{Name: "docker", Args: []string{"image", "inspect", image}})
		if err != nil {
			report.Problems = append(report.Problems, ImageLabel(image)+" is not installed; run local remote setup")
			continue
		}
		report.Images[image] = true
	}
	return report, nil
}

// Setup downloads the exact Guacamole images used by this package.
func Setup(ctx context.Context, runners ...Runner) error {
	var injected Runner
	if len(runners) > 0 {
		injected = runners[0]
	}
	runner := runnerOrDefault(injected)
	if _, err := runner.LookPath("docker"); err != nil {
		return errors.New("local remote setup: Docker CLI is not installed or not in PATH")
	}
	result, err := runner.Run(ctx, Command{Name: "docker", Args: []string{"version", "--format", "{{.Server.Version}}"}})
	if err != nil {
		return commandError("local remote setup: Docker daemon is unavailable", result, err)
	}
	if err := cleanupOrphans(ctx, runner); err != nil {
		return fmt.Errorf("local remote setup: %w", err)
	}
	for _, image := range []string{GuacdImage, GuacamoleImage} {
		result, err = runner.Run(ctx, Command{Name: "docker", Args: []string{"pull", image}})
		if err != nil {
			return commandError("pull "+image, result, err)
		}
	}
	return nil
}

type dockerRuntime struct {
	runner      Runner
	network     string
	guacd       string
	guacamole   string
	secretHex   string
	ownerPID    int
	startedNet  bool
	startedGuac bool
	startedD    bool
}

func (d *dockerRuntime) start(ctx context.Context) (string, error) {
	if d.ownerPID == 0 {
		d.ownerPID = os.Getpid()
	}
	if err := cleanupOrphans(ctx, d.runner); err != nil {
		return "", err
	}
	ownerLabel := ownerPIDLabel + "=" + strconv.Itoa(d.ownerPID)
	result, err := d.runner.Run(ctx, Command{Name: "docker", Args: []string{"network", "create", "--driver", "bridge", "--label", ownershipLabel + "=true", "--label", ownerLabel, d.network}})
	if err != nil {
		return "", commandError("create private Docker network", result, err)
	}
	d.startedNet = true

	result, err = d.runner.Run(ctx, Command{Name: "docker", Args: []string{
		"run", "--detach", "--network", d.network, "--name", d.guacd,
		"--label", ownershipLabel + "=true", "--label", ownerLabel, "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", GuacdImage,
	}})
	if err != nil {
		return "", commandError("start guacd", result, err)
	}
	d.startedD = true

	result, err = d.runner.Run(ctx, Command{
		Name: "docker",
		Args: []string{
			"run", "--detach", "--network", d.network, "--name", d.guacamole,
			"--label", ownershipLabel + "=true", "--label", ownerLabel, "--cap-drop", "ALL",
			"--security-opt", "no-new-privileges", "--publish", "127.0.0.1::8080",
			"--env", "GUACD_HOSTNAME=" + d.guacd, "--env", "JSON_ENABLED=true",
			"--env", "JSON_SECRET_KEY", GuacamoleImage,
		},
		Env: []string{"JSON_SECRET_KEY=" + d.secretHex},
	})
	if err != nil {
		return "", commandError("start Guacamole", result, err)
	}
	d.startedGuac = true

	result, err = d.runner.Run(ctx, Command{Name: "docker", Args: []string{"port", d.guacamole, "8080/tcp"}})
	if err != nil {
		return "", commandError("discover Guacamole loopback port", result, err)
	}
	return parsePublishedLoopback(result.Stdout)
}

func parsePublishedLoopback(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		host, port, err := net.SplitHostPort(strings.TrimSpace(line))
		if err != nil || host != "127.0.0.1" {
			continue
		}
		parsed, err := strconv.Atoi(port)
		if err == nil && parsed > 0 && parsed <= 65535 {
			return "http://127.0.0.1:" + port, nil
		}
	}
	return "", fmt.Errorf("Docker did not publish Guacamole on 127.0.0.1: %q", strings.TrimSpace(output))
}

func (d *dockerRuntime) close(ctx context.Context) error {
	var errs []error
	if d.startedGuac {
		result, err := d.runner.Run(ctx, Command{Name: "docker", Args: []string{"rm", "--force", d.guacamole}})
		errs = append(errs, commandError("remove Guacamole container", result, err))
	}
	if d.startedD {
		result, err := d.runner.Run(ctx, Command{Name: "docker", Args: []string{"rm", "--force", d.guacd}})
		errs = append(errs, commandError("remove guacd container", result, err))
	}
	if d.startedNet {
		result, err := d.runner.Run(ctx, Command{Name: "docker", Args: []string{"network", "rm", d.network}})
		errs = append(errs, commandError("remove private Docker network", result, err))
	}
	return joinErrors(errs...)
}
