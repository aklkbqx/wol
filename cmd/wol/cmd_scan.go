package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	config "github.com/aklkbqx/wol/internal/networkconfig"
	"github.com/aklkbqx/wol/internal/scanner"
	"github.com/aklkbqx/wol/internal/ui"
)

func runScan(args []string) int {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	envFile := flags.String("env-file", envString("WOL_ENV_FILE", ".wol.env"), "optional network settings file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	fmt.Println(ui.RenderHeader("WOL NETWORK SCANNER", "Checking configured LAN, ZeroTier, and local targets..."))

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
		fmt.Println(ui.StyleDanger.Render("✖ Environment configuration error: " + err.Error()))
		return 1
	}
	targets, err := scanner.ScanAllWithEnv(cwd, defaults)
	if err != nil {
		fmt.Println(ui.StyleDanger.Render("✖ Scan configuration error: " + err.Error()))
		return 1
	}
	if len(targets) == 0 {
		fmt.Println(ui.StyleWarning.Render("⚠️ No active devices or hosts discovered."))
		return 0
	}

	var rows [][]string
	for _, t := range targets {
		statusBadge := ui.Badge(t.Status, strings.ToUpper(t.Status))
		sshBadge := ui.Badge("danger", "SSH: NO")
		if t.SSHReachable {
			sshBadge = ui.Badge("success", "SSH: OK")
		}

		rows = append(rows, []string{
			fmt.Sprintf("[%s] %s", strings.ToUpper(string(t.Type)), t.Name),
			fmt.Sprintf("%s  %s  %s", statusBadge, sshBadge, ui.StyleMuted.Render(t.Details)),
		})
	}

	localIP := scanner.GetLocalIP()
	title := fmt.Sprintf("DISCOVERED TARGETS (LOCAL IP: %s)", localIP)
	fmt.Println(ui.RenderBox(title, rows))
	return 0
}
