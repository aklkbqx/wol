package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aklkbqx/wol/internal/localremote"
	"github.com/aklkbqx/wol/internal/remoteflow"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
)

type remoteManager interface {
	Open(context.Context, store.Device, store.RemoteProfile, bool) (string, error)
	Close() error
}

var newRemoteManager = func(repository *store.Store) remoteManager {
	return remoteflow.New(repository, remoteopen.Open)
}

var waitForRemoteStop = func(ctx context.Context) { <-ctx.Done() }

func runRemote(arguments []string) int {
	if len(arguments) > 0 {
		switch strings.ToLower(arguments[0]) {
		case "configure":
			return runRemoteConfigure(arguments[1:])
		case "clear":
			return runRemoteClear(arguments[1:])
		case "doctor":
			return runRemoteDoctor(arguments[1:])
		case "setup":
			return runRemoteSetup(arguments[1:])
		}
	}

	flags := flagSet("remote")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	noWake := flags.Bool("no-wake", false, "do not wake an unreachable machine")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		printRemoteUsage()
		return 2
	}

	repository, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer repository.Close()
	profile, err := repository.GetRemoteProfile(context.Background(), device.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "local remote for %s is not configured; run: wol remote configure --protocol rdp %q\n", device.Name, device.Name)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager := newRemoteManager(repository)
	defer manager.Close()
	url, err := manager.Open(ctx, device, profile, !*noWake)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wake & remote %s: %v\n", device.Name, err)
		return 3
	}
	fmt.Printf("Local remote ready for %s\n%s\nPress Ctrl+C to close the session.\n", device.Name, url)
	waitForRemoteStop(ctx)
	return 0
}

func runRemoteConfigure(arguments []string) int {
	flags := flagSet("remote configure")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	protocol := flags.String("protocol", "rdp", "remote protocol: rdp, vnc, or ssh")
	host := flags.String("host", "", "remote host (defaults to the machine IP)")
	port := flags.Int("port", 0, "remote service port")
	verifyPort := flags.Int("verify-port", 0, "power-check port (defaults to service port)")
	username := flags.String("username", "", "optional username hint; passwords are never stored")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wol remote configure [options] <machine>")
		return 2
	}
	repository, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer repository.Close()
	if strings.TrimSpace(*host) == "" {
		*host = device.IPAddress
	}
	if *port == 0 {
		*port = defaultRemotePort(*protocol)
	}
	if *verifyPort == 0 {
		*verifyPort = *port
	}
	profile, err := repository.UpsertRemoteProfile(context.Background(), store.RemoteProfile{
		DeviceID: device.ID, Protocol: *protocol, Host: *host, Port: *port,
		VerifyPort: *verifyPort, UsernameHint: *username, Mode: "browser-local", Enabled: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure local remote for %s: %v\n", device.Name, err)
		return 2
	}
	fmt.Printf("Configured localhost remote for %s (%s %s:%d).\n", device.Name, profile.Protocol, profile.Host, profile.Port)
	return 0
}

func runRemoteClear(arguments []string) int {
	flags := flagSet("remote clear")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wol remote clear [--db path] <machine>")
		return 2
	}
	repository, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer repository.Close()
	if err := repository.DeleteRemoteProfile(context.Background(), device.ID); err != nil {
		fmt.Fprintf(os.Stderr, "clear local remote for %s: %v\n", device.Name, err)
		return 1
	}
	fmt.Printf("Cleared localhost remote profile for %s.\n", device.Name)
	return 0
}

func runRemoteDoctor(arguments []string) int {
	if len(arguments) != 0 {
		fmt.Fprintln(os.Stderr, "usage: wol remote doctor")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	report, err := localremote.Doctor(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Docker CLI: %s\nDocker daemon: %s\n", readiness(report.DockerCLI), readiness(report.DockerDaemon))
	fmt.Printf("%s: %s\n%s: %s\n", localremote.ImageLabel(localremote.GuacdImage), readiness(report.Images[localremote.GuacdImage]), localremote.ImageLabel(localremote.GuacamoleImage), readiness(report.Images[localremote.GuacamoleImage]))
	for _, problem := range report.Problems {
		fmt.Println("- " + problem)
	}
	if !report.Ready() {
		return 1
	}
	return 0
}

func runRemoteSetup(arguments []string) int {
	if len(arguments) != 0 {
		fmt.Fprintln(os.Stderr, "usage: wol remote setup")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	fmt.Println("Installing pinned localhost remote images...")
	if err := localremote.Setup(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Local remote runtime is ready.")
	return 0
}

func printRemoteUsage() {
	fmt.Fprintln(os.Stderr, "usage: wol remote [--no-wake] <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote configure [options] <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote clear <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote doctor | setup")
}

func defaultRemotePort(protocol string) int {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "vnc":
		return 5900
	case "ssh":
		return 22
	default:
		return 3389
	}
}

func readiness(ok bool) string {
	if ok {
		return "READY"
	}
	return "MISSING"
}

func remoteDevice(databasePath, target string) (*store.Store, store.Device, int) {
	repository, err := store.Open(databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open local inventory")
		return nil, store.Device{}, 1
	}
	device, err := findStoredDevice(repository, target)
	if err != nil {
		repository.Close()
		fmt.Fprintln(os.Stderr, err)
		return nil, store.Device{}, 2
	}
	return repository, device, 0
}
