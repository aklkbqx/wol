package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	"golang.org/x/term"
)

const appVersion = buildinfo.Version
const appCredit = buildinfo.Credit

func main() {
	if len(os.Args) < 2 {
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			usage()
			os.Exit(2)
		}
		os.Exit(runWakeDesk(nil))
	}
	switch os.Args[1] {
	case "tui":
		exitCode := runWakeDesk(os.Args[2:])
		os.Exit(exitCode)
	case "scan":
		exitCode := runScan(os.Args[2:])
		os.Exit(exitCode)
	case "doctor":
		exitCode := runDoctor(os.Args[2:])
		os.Exit(exitCode)
	case "status":
		exitCode := runStatus(os.Args[2:])
		os.Exit(exitCode)
	case "wake":
		exitCode := runWake(os.Args[2:])
		os.Exit(exitCode)
	case "remote":
		os.Exit(runRemote(os.Args[2:]))
	case "import":
		os.Exit(runImport(os.Args[2:]))
	case "export":
		os.Exit(runExport(os.Args[2:]))
	case "version":
		printVersion(os.Stdout)
	default:
		usage()
		os.Exit(2)
	}
}

func envString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func runWake(arguments []string) int {
	flags := flag.NewFlagSet("wake", flag.ContinueOnError)
	destination := flags.String("destination", "255.255.255.255", "IPv4 destination or broadcast address")
	port := flags.Int("port", 9, "UDP port")
	interfaceName := flags.String("interface", "", "network interface name")
	repeat := flags.Int("repeat", 3, "number of packets")
	interval := flags.Duration("interval", 200*time.Millisecond, "delay between packets")
	routerHost := flags.String("router", "", "optional router SSH host for remote etherwake relay (e.g. router or 198.51.100.1)")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path for a stored device target")
	deviceTarget := flags.String("device", "", "stored device name or id (alternative to a positional MAC)")
	verify := flags.Bool("verify", false, "wait for the stored device TCP verification port after waking")
	verifyPort := flags.Int("verify-port", 0, "override stored device verification port")
	force := flags.Bool("force", false, "wake a stored device even when it is disabled")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() > 1 || (flags.NArg() == 0 && strings.TrimSpace(*deviceTarget) == "") {
		fmt.Fprintln(os.Stderr, "wake requires one MAC address or --device name/id")
		return 2
	}
	target := strings.TrimSpace(*deviceTarget)
	if target == "" {
		target = flags.Arg(0)
	}
	mac, parseErr := wol.ParseMAC(target)

	if parseErr != nil {
		dataStore, err := store.Open(*databasePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open SQLite database: %v\n", err)
			return 1
		}
		defer dataStore.Close()
		device, err := findStoredDevice(dataStore, target)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		result, err := wake.NewService(dataStore, wake.Hooks{}).WakeDevice(context.Background(), device.ID, wake.Options{Repeat: *repeat, Interval: *interval, Verify: *verify, VerifyPort: *verifyPort, Force: *force})
		if err != nil {
			fmt.Fprintf(os.Stderr, "wake %s failed: %v\n", device.Name, err)
			if result.Attempt.ID != "" {
				fmt.Fprintf(os.Stderr, "attempt %s recorded as %s\n", result.Attempt.ID, result.Attempt.PacketStatus)
			}
			return 3
		}
		fmt.Printf("woke %s via %s: %d packet(s), %s\n", result.Device.Name, result.Route.Name, result.Attempt.Packets, result.Attempt.PacketStatus)
		if result.Attempt.VerificationStatus != "not_requested" {
			fmt.Printf("verification: %s\n", result.Attempt.VerificationStatus)
		}
		return 0
	}
	if *routerHost != "" {
		fmt.Printf("Relaying etherwake via router SSH host [%s] for MAC %s...\n", *routerHost, target)
		out, err := wake.SendEtherwake(context.Background(), store.WakeRelay{Name: *routerHost, Address: *routerHost, Port: 22, Transport: "ssh_etherwake", Interface: "br-lan", Enabled: true}, mac)
		if err != nil {
			fmt.Fprintf(os.Stderr, "router relay failed: %v\n", err)
			return 3
		}
		if out.Detail != "" {
			fmt.Println(out.Detail)
		}
		fmt.Printf("successfully triggered etherwake on router [%s] for %s\n", *routerHost, target)
		return 0
	}

	ip := net.ParseIP(*destination)
	if ip == nil || ip.To4() == nil {
		fmt.Fprintln(os.Stderr, "destination must be a valid IPv4 address")
		return 2
	}
	result, err := wol.Send(context.Background(), wol.SendRequest{MAC: mac, Destination: ip.To4(), Port: *port, Interface: *interfaceName, Repeat: *repeat, Interval: *interval})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	fmt.Printf("sent %d packets (%d bytes) to %s:%d\n", result.Packets, result.Bytes, result.Destination, result.Port)
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, "WOL WAKE DESK - Standalone Wake-on-LAN Console (v%s)\n", appVersion)
	fmt.Fprintf(os.Stderr, "Credit: %s\n", appCredit)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: wol [command] [arguments]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "No command opens the standalone Wake Desk TUI.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  wol tui          Open the local Wake Desk")
	fmt.Fprintln(os.Stderr, "  wol wake         Wake a stored machine or MAC address")
	fmt.Fprintln(os.Stderr, "  wol remote       Open or configure a machine's remote session")
	fmt.Fprintln(os.Stderr, "  wol status       Check a stored machine's power state")
	fmt.Fprintln(os.Stderr, "  wol scan         Discover local network targets")
	fmt.Fprintln(os.Stderr, "  wol doctor       Check the local wake toolchain")
	fmt.Fprintln(os.Stderr, "  wol import       Import a portable inventory JSON file")
	fmt.Fprintln(os.Stderr, "  wol export       Export portable inventory JSON")
	fmt.Fprintln(os.Stderr, "  wol version      Print version and credit")
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "%s\nCredit: %s\n", appVersion, appCredit)
}
