package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/tui"
)

func runWakeDesk(arguments []string) int {
	flags := flagSet("tui")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wol tui [--db path]")
		return 2
	}
	if err := tui.RunWakeDesk(*databasePath); err != nil {
		fmt.Fprintln(os.Stderr, "wake desk: could not open local inventory")
		return 1
	}
	return 0
}

// flagSet keeps command flag parsing consistent while allowing tests and
// callers to inject the command name without changing behavior.
func flagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

func findStoredDevice(dataStore *store.Store, target string) (store.Device, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return store.Device{}, fmt.Errorf("device name or id is required")
	}
	if device, err := dataStore.GetDevice(context.Background(), target); err == nil {
		return device, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Device{}, err
	}
	devices, err := dataStore.ListDevices(context.Background())
	if err != nil {
		return store.Device{}, err
	}
	for _, device := range devices {
		if strings.EqualFold(device.Name, target) {
			return device, nil
		}
	}
	return store.Device{}, fmt.Errorf("machine %q not found in local inventory", target)
}
