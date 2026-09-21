package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
)

func runBatch(args []string) int {
	if len(args) == 0 || (args[0] != "check" && args[0] != "wake") {
		fmt.Fprintln(os.Stderr, "usage: wol batch check|wake [--site NAME | --all | machine ...] [--json] [--yes] [--no-input]")
		return 2
	}
	kind := args[0]
	flags := flagSet("batch " + kind)
	db := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "inventory database")
	site := flags.String("site", "", "site name or ID")
	all := flags.Bool("all", false, "select every machine")
	jsonOut := flags.Bool("json", false, "emit one JSON result per line")
	yes := flags.Bool("yes", false, "confirm sending wake packets to selected machines")
	flags.Bool("no-input", false, "never prompt (batch commands never prompt)")
	concurrency := flags.Int("concurrency", 16, "maximum simultaneous operations (1–32)")
	if flags.Parse(args[1:]) != nil || *concurrency < 1 || *concurrency > 32 {
		return 2
	}
	selectors := 0
	if *all {
		selectors++
	}
	if *site != "" {
		selectors++
	}
	if flags.NArg() > 0 {
		selectors++
	}
	if selectors != 1 {
		fmt.Fprintln(os.Stderr, "select exactly one of --all, --site NAME, or machine names")
		return 2
	}
	repo, err := store.Open(*db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer repo.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	devices, err := repo.ListDevices(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sites, err := repo.ListSites(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	selected := []store.Device{}
	if *site != "" {
		id := ""
		for _, s := range sites {
			if s.ID == *site || strings.EqualFold(s.Name, *site) {
				id = s.ID
			}
		}
		if id == "" {
			fmt.Fprintln(os.Stderr, "site not found")
			return 2
		}
		for _, d := range devices {
			if d.SiteID == id {
				selected = append(selected, d)
			}
		}
	} else if *all {
		selected = devices
	} else {
		seen := map[string]bool{}
		for _, name := range flags.Args() {
			d, err := findStoredDevice(repo, name)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			if !seen[d.ID] {
				selected = append(selected, d)
				seen[d.ID] = true
			}
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "no matching machines")
		return 2
	}
	if kind == "wake" && !*yes {
		fmt.Fprintf(os.Stderr, "Wake preview: %d machines. Re-run with --yes to send packets.\n", len(selected))
		for _, d := range selected {
			fmt.Fprintf(os.Stderr, "  %s  site=%s\n", d.Name, d.SiteID)
		}
		return 2
	}
	profiles, err := repo.ListRemoteProfiles(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	op := fleet.Check(presence.NewDetector(), profiles)
	if kind == "wake" {
		op = fleet.Wake(wakeservice.NewService(repo, wakeservice.Hooks{}))
	}
	good, bad := 0, 0
	for r := range fleet.Run(ctx, selected, sites, *concurrency, op) {
		if *jsonOut {
			if err := json.NewEncoder(os.Stdout).Encode(r); err != nil {
				return 1
			}
		} else {
			fmt.Printf("%s  %s  %s  %s\n", r.Name, r.Status, r.Method, r.Message)
		}
		if r.Status == "online" || r.Status == "sent" {
			good++
		} else {
			bad++
		}
	}
	if ctx.Err() != nil {
		return 130
	}
	if bad > 0 && good > 0 {
		return 3
	}
	if bad > 0 {
		return 1
	}
	return 0
}
