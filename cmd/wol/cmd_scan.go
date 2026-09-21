package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	config "github.com/aklkbqx/wol/internal/networkconfig"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/ui"
)

func runScan(args []string) int {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	envFile := flags.String("env-file", envString("WOL_ENV_FILE", ".wol.env"), "optional network settings file")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "inventory database path")
	addUnknown := flags.Bool("add", false, "add unknown LAN neighbors to the local inventory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	fmt.Println(ui.RenderHeader("LAN SCAN", "ARP neighbors on local ethernet/wifi interfaces"))

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	neighbors, err := scanner.ScanLAN(ctx)
	if err != nil {
		fmt.Println(ui.StyleDanger.Render("✖ LAN neighbor scan failed: " + err.Error()))
		return 1
	}

	repository, err := store.Open(*databasePath)
	if err != nil {
		fmt.Println(ui.StyleDanger.Render("✖ could not open local inventory"))
		return 1
	}
	defer repository.Close()
	devices, err := repository.ListDevices(ctx)
	if err != nil {
		fmt.Println(ui.StyleDanger.Render("✖ list inventory: " + err.Error()))
		return 1
	}

	known, moved, unknown := scanner.ClassifyLAN(devices, neighbors)
	if _, err := scanner.SyncDeviceIPs(ctx, repository, devices, neighbors); err != nil {
		fmt.Println(ui.StyleDanger.Render("✖ sync inventory: " + err.Error()))
		return 1
	}

	var rows [][]string
	for _, host := range known {
		rows = append(rows, lanRow(host.Neighbor.IP, host.Neighbor.MAC, host.Device.Name, "known"))
	}
	for _, host := range moved {
		rows = append(rows, lanRow(host.Neighbor.IP, host.Neighbor.MAC, host.Device.Name, "updated-ip"))
	}
	for _, neighbor := range unknown {
		rows = append(rows, lanRow(neighbor.IP, neighbor.MAC, "", "new"))
	}
	if len(rows) == 0 {
		fmt.Println(ui.StyleWarning.Render("No LAN neighbors with complete ARP entries."))
	} else {
		fmt.Println(ui.RenderBox(fmt.Sprintf("LAN NEIGHBORS (%s)", scanner.GetLocalIP()), rows))
	}

	if *addUnknown && len(unknown) > 0 {
		added := 0
		for _, neighbor := range unknown {
			// Each host gets its own budget; a large LAN must not exhaust the
			// scan timeout and silently skip the remaining database writes.
			addCtx, addCancel := context.WithTimeout(context.Background(), 3*time.Second)
			name := scanner.UniqueDeviceName(devices, scanner.SuggestName(neighbor.IP, neighbor.MAC))
			platform, verifyPort := scanner.GuessIdentity(addCtx, neighbor.IP)
			item, err := repository.CreateDevice(addCtx, store.Device{
				Name:             name,
				MACAddress:       neighbor.MAC,
				IPAddress:        neighbor.IP,
				BroadcastAddress: scanner.BroadcastOf(neighbor.IP),
				Port:             9,
				Interface:        neighbor.Iface,
				Platform:         platform,
				DeviceType:       "unknown",
				WakeStrategy:     "broadcast",
				VerifyPort:       verifyPort,
				Enabled:          true,
			})
			addCancel()
			if err != nil {
				fmt.Println(ui.StyleDanger.Render("✖ add " + neighbor.IP + ": " + err.Error()))
				continue
			}
			devices = append(devices, item)
			added++
			fmt.Printf("Added %s  %s  %s  verify %d\n", item.Name, item.IPAddress, item.MACAddress, item.VerifyPort)
		}
		fmt.Printf("Added %d machine(s) to inventory.\n", added)
		if added != len(unknown) {
			return 1
		}
	} else if len(unknown) > 0 {
		fmt.Println(ui.StyleMuted.Render("New hosts are not stored yet. Re-run with --add to import them."))
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	envPath := *envFile
	if !filepath.IsAbs(envPath) {
		envPath = filepath.Join(cwd, envPath)
	}
	defaults, err := config.LoadEnvFile(envPath)
	if err != nil {
		return 0
	}
	targets, err := scanner.ScanNetworkTargetsWithEnv(cwd, defaults)
	if err != nil || len(targets) == 0 {
		return 0
	}
	var extra [][]string
	for _, t := range targets {
		statusBadge := ui.Badge(t.Status, strings.ToUpper(t.Status))
		extra = append(extra, []string{
			fmt.Sprintf("[%s] %s", strings.ToUpper(string(t.Type)), t.Name),
			fmt.Sprintf("%s  %s", statusBadge, ui.StyleMuted.Render(t.Details)),
		})
	}
	fmt.Println(ui.RenderBox("CONFIGURED TARGETS", extra))
	return 0
}

func lanRow(ip, mac, name, state string) []string {
	label := ip
	if name != "" {
		label = name + "  " + ip
	}
	return []string{label, fmt.Sprintf("%s  %s", ui.Badge("success", strings.ToUpper(state)), ui.StyleMuted.Render(mac))}
}
