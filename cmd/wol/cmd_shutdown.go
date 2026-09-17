package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/power"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/ui"
)

func runShutdown(arguments []string) int {
	if len(arguments) > 0 {
		switch strings.ToLower(arguments[0]) {
		case "configure", "config":
			return runShutdownConfigure(arguments[1:])
		case "cancel":
			return runShutdownCancel(arguments[1:])
		case "status":
			return runShutdownStatus(arguments[1:])
		}
	}

	flags := flag.NewFlagSet("shutdown", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	delayStr := flags.String("delay", "", "delay before shutdown (e.g. 15m, 30m, 1h, 1800s)")
	now := flags.Bool("now", false, "shutdown immediately (default)")
	force := flags.Bool("force", false, "force running applications to close")
	user := flags.String("user", "", "override SSH username")
	port := flags.Int("port", 0, "override SSH port")
	key := flags.String("key", "", "override SSH private key path")
	platform := flags.String("platform", "", "override target platform (windows, linux, darwin)")
	useSudo := flags.Bool("sudo", false, "use sudo for linux/darwin shutdown")
	cancel := flags.Bool("cancel", false, "abort a scheduled shutdown")

	targetName, err := parseMachineAndFlags(arguments, flags)
	if err != nil {
		return 2
	}
	if targetName == "" {
		printShutdownUsage()
		return 2
	}
	dataStore, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open SQLite database: %v\n", err)
		return 1
	}
	defer dataStore.Close()

	device, err := findStoredDevice(dataStore, targetName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	target, err := resolvePowerTarget(dataStore, device, *user, *port, *key, *platform, *useSudo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve power target: %v\n", err)
		return 2
	}

	var delay time.Duration
	if *cancel {
		// Cancel operation
	} else if strings.TrimSpace(*delayStr) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(*delayStr))
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid delay %q (expected format like 15m, 1h, 30s): %v\n", *delayStr, err)
			return 2
		}
		if parsed < 0 {
			fmt.Fprintf(os.Stderr, "invalid delay %q: delay cannot be negative\n", *delayStr)
			return 2
		}
		delay = parsed
	} else if !*now && delay == 0 {
		// Default is immediate
		delay = 0
	}

	svc := power.NewService(nil)
	ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()

	req := power.Request{
		Target: target,
		Delay:  delay,
		Cancel: *cancel,
		Force:  *force,
	}

	result, execErr := svc.Execute(ctx, req)

	// Record attempt in store
	actionStr := string(result.Action)
	statusStr := "sent"
	errMsg := ""
	if execErr != nil {
		statusStr = "failed"
		errMsg = execErr.Error()
	}

	_, _ = dataStore.RecordPowerAttempt(context.Background(), store.PowerAttempt{
		DeviceID:     device.ID,
		DeviceName:   device.Name,
		Action:       actionStr,
		DelaySeconds: int(delay.Seconds()),
		Status:       statusStr,
		Message:      errMsg,
	})

	if execErr != nil {
		fmt.Fprintf(os.Stderr, "%s\n", ui.StyleDanger.Render(fmt.Sprintf("✖ Power command failed for %s: %v", device.Name, execErr)))
		return 3
	}

	switch result.Action {
	case power.ActionCancel:
		fmt.Println(ui.StyleSuccess.Render(fmt.Sprintf("✔ Cancelled pending shutdown for %s (%s)", device.Name, target.Host)))
	case power.ActionSchedule:
		fmt.Println(ui.StyleSuccess.Render(fmt.Sprintf("✔ Scheduled shutdown for %s (%s) in %s (at %s)", device.Name, target.Host, delay, result.ScheduledTime.Format("15:04:05"))))
	default:
		fmt.Println(ui.StyleSuccess.Render(fmt.Sprintf("✔ Sent immediate shutdown command to %s (%s)", device.Name, target.Host)))
	}

	return 0
}

func runShutdownCancel(arguments []string) int {
	return runShutdown(append([]string{"--cancel"}, arguments...))
}

func runShutdownConfigure(arguments []string) int {
	flags := flag.NewFlagSet("shutdown configure", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	user := flags.String("user", "", "SSH username")
	port := flags.Int("port", 22, "SSH port")
	key := flags.String("key", "", "SSH private key path")
	platform := flags.String("platform", "windows", "Target platform: windows, linux, darwin")
	useSudo := flags.Bool("sudo", false, "Use sudo for shutdown commands")

	machine, err := parseMachineAndFlags(arguments, flags)
	if err != nil {
		return 2
	}
	if machine == "" {
		fmt.Fprintln(os.Stderr, "usage: wol shutdown configure <machine> [--user <user>] [--port 22] [--key <path>] [--platform <windows|linux|darwin>] [--sudo]")
		return 2
	}

	dataStore, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open SQLite database: %v\n", err)
		return 1
	}
	defer dataStore.Close()

	device, err := findStoredDevice(dataStore, machine)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	profile, err := dataStore.UpsertPowerProfile(context.Background(), store.PowerProfile{
		DeviceID: device.ID,
		SSHUser:  *user,
		SSHPort:  *port,
		SSHKey:   *key,
		Platform: *platform,
		UseSudo:  *useSudo,
		Enabled:  true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "save power profile: %v\n", err)
		return 1
	}

	fmt.Printf("Power profile for %s configured successfully:\n  User: %s\n  Port: %d\n  Platform: %s\n  UseSudo: %v\n",
		device.Name, profile.SSHUser, profile.SSHPort, profile.Platform, profile.UseSudo)
	return 0
}

func runShutdownStatus(arguments []string) int {
	flags := flag.NewFlagSet("shutdown status", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	limit := flags.Int("limit", 10, "number of recent attempts to display")

	if err := flags.Parse(arguments); err != nil {
		return 2
	}

	dataStore, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open SQLite database: %v\n", err)
		return 1
	}
	defer dataStore.Close()

	attempts, err := dataStore.ListPowerAttempts(context.Background(), *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list power attempts: %v\n", err)
		return 1
	}

	if len(attempts) == 0 {
		fmt.Println("No power operation history recorded.")
		return 0
	}

	fmt.Println(ui.RenderHeader("RECENT POWER OPERATIONS", "Historical shutdown, schedule, and cancel events"))
	var rows [][]string
	for _, a := range attempts {
		statusBadge := ui.Badge("success", strings.ToUpper(a.Status))
		if a.Status != "sent" {
			statusBadge = ui.Badge("danger", strings.ToUpper(a.Status))
		}
		detail := fmt.Sprintf("action: %s", a.Action)
		if a.DelaySeconds > 0 {
			detail += fmt.Sprintf(" (%ds delay)", a.DelaySeconds)
		}
		if a.Message != "" {
			detail += fmt.Sprintf(" · %s", a.Message)
		}
		created := a.CreatedAt
		if len(created) >= 19 {
			created = created[:19]
		}
		rows = append(rows, []string{
			fmt.Sprintf("[%s] %s", created, a.DeviceName),
			fmt.Sprintf("%s  %s", statusBadge, ui.StyleMuted.Render(detail)),
		})
	}
	fmt.Println(ui.RenderBox("POWER HISTORY", rows))
	return 0
}

func resolvePowerTarget(dataStore *store.Store, device store.Device, userFlag string, portFlag int, keyFlag string, platformFlag string, sudoFlag bool) (power.Target, error) {
	host := strings.TrimSpace(device.IPAddress)
	if host == "" {
		return power.Target{}, fmt.Errorf("device %s has no IP address configured", device.Name)
	}

	target := power.Target{
		DeviceID:   device.ID,
		DeviceName: device.Name,
		Host:       host,
		Port:       22,
		Platform:   device.Platform,
	}

	// Try loading stored PowerProfile
	if profile, err := dataStore.GetPowerProfile(context.Background(), device.ID); err == nil && profile.Enabled {
		if profile.SSHUser != "" {
			target.User = profile.SSHUser
		}
		if profile.SSHPort > 0 {
			target.Port = profile.SSHPort
		}
		if profile.SSHKey != "" {
			target.KeyPath = profile.SSHKey
		}
		if profile.Platform != "" {
			target.Platform = profile.Platform
		}
		target.UseSudo = profile.UseSudo
	} else {
		// Fallback: check RemoteProfile for hints
		if rProfile, err := dataStore.GetRemoteProfile(context.Background(), device.ID); err == nil && rProfile.Enabled {
			if rProfile.UsernameHint != "" {
				target.User = rProfile.UsernameHint
			}
			if rProfile.Protocol == "ssh" && rProfile.Port > 0 {
				target.Port = rProfile.Port
			}
		}
	}

	// CLI flags override stored profile
	if strings.TrimSpace(userFlag) != "" {
		target.User = strings.TrimSpace(userFlag)
	}
	if portFlag > 0 {
		target.Port = portFlag
	}
	if strings.TrimSpace(keyFlag) != "" {
		target.KeyPath = strings.TrimSpace(keyFlag)
	}
	if strings.TrimSpace(platformFlag) != "" {
		target.Platform = strings.TrimSpace(platformFlag)
	}
	if sudoFlag {
		target.UseSudo = true
	}

	if target.Platform == "" || target.Platform == "unknown" {
		target.Platform = "windows"
	}

	return target, nil
}

func printShutdownUsage() {
	fmt.Fprintln(os.Stderr, "Usage: wol shutdown [command] [arguments]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  wol shutdown <machine>                 Shutdown the machine immediately")
	fmt.Fprintln(os.Stderr, "  wol shutdown <machine> --delay 30m     Schedule shutdown in 30 minutes")
	fmt.Fprintln(os.Stderr, "  wol shutdown cancel <machine>          Abort a pending scheduled shutdown")
	fmt.Fprintln(os.Stderr, "  wol shutdown config <machine> [flags]  Configure SSH credentials and platform")
	fmt.Fprintln(os.Stderr, "  wol shutdown status                    View recent power operation history")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --delay <duration>  Delay before shutdown (e.g. 15m, 30m, 1h, 1800s)")
	fmt.Fprintln(os.Stderr, "  --now               Shutdown immediately")
	fmt.Fprintln(os.Stderr, "  --force             Force applications to close without prompt")
	fmt.Fprintln(os.Stderr, "  --cancel            Abort pending scheduled shutdown")
	fmt.Fprintln(os.Stderr, "  --user <name>       SSH username")
	fmt.Fprintln(os.Stderr, "  --port <number>     SSH port (default 22)")
	fmt.Fprintln(os.Stderr, "  --key <path>        Path to private SSH key")
	fmt.Fprintln(os.Stderr, "  --platform <os>     Target OS (windows, linux, darwin)")
	fmt.Fprintln(os.Stderr, "  --sudo              Execute command with sudo on Linux/Darwin")
	fmt.Fprintln(os.Stderr, "  --db <path>         SQLite database path")
}

func parseMachineAndFlags(arguments []string, flags *flag.FlagSet) (string, error) {
	if len(arguments) == 0 {
		return "", nil
	}
	if !strings.HasPrefix(arguments[0], "-") {
		machine := arguments[0]
		if err := flags.Parse(arguments[1:]); err != nil {
			return "", err
		}
		if machine == "" && flags.NArg() > 0 {
			machine = flags.Arg(0)
		}
		return machine, nil
	}
	if err := flags.Parse(arguments); err != nil {
		return "", err
	}
	machine := ""
	if flags.NArg() > 0 {
		machine = flags.Arg(0)
	}
	return machine, nil
}
