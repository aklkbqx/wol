package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aklkbqx/wol/internal/localremote"
	"github.com/aklkbqx/wol/internal/moonlight"
	"github.com/aklkbqx/wol/internal/remoteflow"
	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/sunshine"
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
		case "pair":
			return runRemotePair(arguments[1:])
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
		fmt.Fprintf(os.Stderr, "local remote for %s is not configured; run: wol remote configure --protocol sunshine %q (or --protocol rdp)\n", device.Name, device.Name)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager := newRemoteManager(repository)
	defer manager.Close()
	msg, err := manager.Open(ctx, device, profile, !*noWake)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wake & remote %s: %v\n", device.Name, err)
		return 3
	}

	if profile.Mode == "native-moonlight" || profile.Protocol == "sunshine" {
		fmt.Printf("%s\nPress Ctrl+C to disconnect.\n", msg)
		waitForRemoteStop(ctx)
		return 0
	}

	fmt.Printf("Local sign-in opened for %s\n%s\nCredentials stay in memory only. Press Ctrl+C to close the session.\n", device.Name, msg)
	waitForRemoteStop(ctx)
	return 0
}

func runRemotePair(arguments []string) int {
	flags := flagSet("remote pair")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	pin := flags.String("pin", "", "optional 4-digit PIN for moonlight pair / Sunshine")
	clientName := flags.String("name", "wol-client", "client name to register with Sunshine")
	user := flags.String("user", envString("SUNSHINE_USER", ""), "Sunshine admin username (or SUNSHINE_USER)")
	pass := flags.String("pass", envString("SUNSHINE_PASS", ""), "Sunshine admin password (or SUNSHINE_PASS)")
	port := flags.Int("port", sunshine.DefaultAdminPort, "Sunshine admin HTTPS port")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wol remote pair [--pin 4-digit] [--name client-name] <machine>")
		return 2
	}
	repository, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer repository.Close()
	host := device.IPAddress
	if profile, err := repository.GetRemoteProfile(context.Background(), device.ID); err == nil && profile.Host != "" {
		host = profile.Host
	}

	moonlightClient, errMoonlight := moonlight.Detect()
	if errMoonlight == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := moonlightClient.Pair(ctx, host, *pin); err != nil {
			fmt.Fprintf(os.Stderr, "pair Moonlight with %s (%s): %v\n", device.Name, host, err)
			return 3
		}
		fmt.Printf("Paired Moonlight with %s (%s).\n", device.Name, host)
		return 0
	}

	if strings.TrimSpace(*pin) == "" {
		fmt.Fprintln(os.Stderr, "Moonlight is not installed. Install it, or pass --pin with SUNSHINE_USER/SUNSHINE_PASS to pair through Sunshine.")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := sunshine.NewClient(host, *port, *user, *pass)
	if err := client.Pair(ctx, *pin, *clientName); err != nil {
		fmt.Fprintf(os.Stderr, "pair Moonlight with Sunshine on %s (%s:%d): %v\n", device.Name, host, *port, err)
		return 3
	}
	fmt.Printf("Paired client %s with Sunshine on %s (%s:%d).\n", *clientName, device.Name, host, *port)
	return 0
}

func runRemoteConfigure(arguments []string) int {
	flags := flagSet("remote configure")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	protocol := flags.String("protocol", "sunshine", "remote protocol: sunshine, rdp, vnc, or ssh")
	host := flags.String("host", "", "remote host (defaults to the machine IP)")
	port := flags.Int("port", 0, "remote service port (defaults: 47989 for sunshine, 3389 for rdp, 5900 for vnc, 22 for ssh)")
	verifyPort := flags.Int("verify-port", 0, "power-check port (defaults to service port)")
	username := flags.String("username", "", "optional username hint; passwords are never stored")
	domain := flags.String("domain", "", "optional RDP domain hint")
	certificate := flags.String("certificate", "strict", "RDP certificate policy: strict or trust-local")
	mode := flags.String("mode", "", "remote mode: native-moonlight or browser-local")
	app := flags.String("app", "Desktop", "Sunshine application name")
	fps := flags.Int("fps", 0, "Moonlight streaming frame rate (e.g. 60, 120)")
	res := flags.String("res", "", "Moonlight streaming resolution (e.g. 1920x1080, 2560x1440)")
	bitrate := flags.Int("bitrate", 0, "Moonlight bitrate in kbps (e.g. 50000)")

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
	if *mode == "" {
		if strings.EqualFold(*protocol, "sunshine") {
			*mode = "native-moonlight"
		} else {
			*mode = "browser-local"
		}
	}
	profile, err := repository.UpsertRemoteProfile(context.Background(), store.RemoteProfile{
		DeviceID: device.ID, Protocol: *protocol, Host: *host, Port: *port,
		VerifyPort: *verifyPort, UsernameHint: *username, DomainHint: *domain,
		CertificatePolicy: *certificate, Mode: *mode, AppName: *app,
		FPS: *fps, Resolution: *res, BitrateKbps: *bitrate, Enabled: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure local remote for %s: %v\n", device.Name, err)
		return 2
	}
	if profile.Protocol == "sunshine" {
		fmt.Printf("Configured Sunshine remote for %s (%s %s:%d, mode: %s, app: %s, fps: %d, res: %s).\n", device.Name, profile.Protocol, profile.Host, profile.Port, profile.Mode, profile.AppName, profile.FPS, profile.Resolution)
	} else {
		fmt.Printf("Configured localhost remote for %s (%s %s:%d, certificate %s).\n", device.Name, profile.Protocol, profile.Host, profile.Port, profile.CertificatePolicy)
	}
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
	flags := flagSet("remote doctor")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: wol remote doctor [--db path] [machine]")
		return 2
	}

	// 1. Check local Moonlight client
	moonlightClient, errMoonlight := moonlight.Detect()
	moonlightStatus := "READY"
	moonlightDetail := ""
	if errMoonlight == nil {
		moonlightDetail = "(" + moonlightClient.ExecutablePath + ")"
	} else {
		moonlightStatus = "MISSING"
		moonlightDetail = "(install from https://moonlight-stream.org)"
	}
	fmt.Printf("Moonlight Client: %s %s\n", moonlightStatus, moonlightDetail)

	// 2. Check local Docker (for Guacamole fallback)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	report, err := localremote.Doctor(ctx)
	if err == nil {
		fmt.Printf("Docker CLI: %s\nDocker daemon: %s\n", readiness(report.DockerCLI), readiness(report.DockerDaemon))
		fmt.Printf("%s: %s\n%s: %s\n", localremote.ImageLabel(localremote.GuacdImage), readiness(report.Images[localremote.GuacdImage]), localremote.ImageLabel(localremote.GuacamoleImage), readiness(report.Images[localremote.GuacamoleImage]))
		for _, problem := range report.Problems {
			fmt.Println("- " + problem)
		}
	}

	targetReady := true
	if flags.NArg() == 1 {
		repository, device, code := remoteDevice(*databasePath, flags.Arg(0))
		if code != 0 {
			return code
		}
		defer repository.Close()
		targetCtx, targetCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer targetCancel()
		profile, profileErr := repository.GetRemoteProfile(targetCtx, device.ID)
		if profileErr != nil {
			fmt.Printf("Target %s: PROFILE MISSING\n", device.Name)
			targetReady = false
		} else if profile.Protocol == "sunshine" {
			gsReachable := sunshine.Probe(targetCtx, profile.Host, profile.Port)
			adminReachable := sunshine.Probe(targetCtx, profile.Host, sunshine.DefaultAdminPort)
			rtspReachable := sunshine.Probe(targetCtx, profile.Host, sunshine.DefaultRTSPPort)

			state := "REACHABLE"
			if !gsReachable {
				state = "OFFLINE"
				targetReady = false
			}
			fmt.Printf("Target %s: SUNSHINE %s · GameStream:%s Admin:%s RTSP:%s · %dfps %s\n",
				device.Name, state,
				portStatus(gsReachable), portStatus(adminReachable), portStatus(rtspReachable),
				profile.FPS, profile.Resolution)
		} else {
			connection, dialErr := (&net.Dialer{}).DialContext(targetCtx, "tcp", net.JoinHostPort(profile.Host, strconv.Itoa(profile.VerifyPort)))
			if connection != nil {
				_ = connection.Close()
			}
			state := "REACHABLE"
			if dialErr != nil {
				state = "OFFLINE"
				targetReady = false
			}
			fmt.Printf("Target %s: %s %s · certificate %s · %s\n", device.Name, strings.ToUpper(profile.Protocol), state, profile.CertificatePolicy, credentialPrompt(profile.Protocol))
		}
	}
	if !targetReady {
		return 1
	}
	return 0
}

func portStatus(ok bool) string {
	if ok {
		return "OK"
	}
	return "DOWN"
}

func credentialPrompt(protocol string) string {
	switch protocol {
	case "rdp":
		return "browser prompts for username/domain/password"
	case "ssh":
		return "browser prompts for username/password"
	case "sunshine":
		return "moonlight stream"
	default:
		return "browser prompts for password"
	}
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
	fmt.Fprintln(os.Stderr, "       wol remote pair [--pin 4-digit] <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote configure [options] <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote clear <machine>")
	fmt.Fprintln(os.Stderr, "       wol remote doctor [machine] | setup")
}

func defaultRemotePort(protocol string) int {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "vnc":
		return 5900
	case "ssh":
		return 22
	case "sunshine":
		return sunshine.DefaultGameStreamPort
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
