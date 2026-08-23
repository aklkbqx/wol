package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/remoteopen"
	"github.com/aklkbqx/wol/internal/store"
)

var openRemoteURL = remoteopen.Open

func runRemote(arguments []string) int {
	if len(arguments) > 0 {
		switch strings.ToLower(arguments[0]) {
		case "set":
			return runRemoteSet(arguments[1:], false)
		case "clear":
			return runRemoteSet(arguments[1:], true)
		}
	}

	flags := flagSet("remote")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wol remote [--db path] <machine>")
		fmt.Fprintln(os.Stderr, "       wol remote set [--db path] <machine> <http|https URL>")
		fmt.Fprintln(os.Stderr, "       wol remote clear [--db path] <machine>")
		return 2
	}

	dataStore, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer dataStore.Close()
	validated, err := remoteopen.Validate(device.RemoteURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "remote %s is not ready: %v\n", device.Name, err)
		fmt.Fprintf(os.Stderr, "configure it with: wol remote set %q https://example.test/remote/%s\n", device.Name, device.ID)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := openRemoteURL(ctx, validated); err != nil {
		fmt.Fprintf(os.Stderr, "open remote for %s: %v\n", device.Name, err)
		return 3
	}
	fmt.Printf("opened remote for %s\n", device.Name)
	return 0
}

func runRemoteSet(arguments []string, clear bool) int {
	flags := flagSet("remote")
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "SQLite database path")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	want := 2
	if clear {
		want = 1
	}
	if flags.NArg() != want {
		if clear {
			fmt.Fprintln(os.Stderr, "usage: wol remote clear [--db path] <machine>")
		} else {
			fmt.Fprintln(os.Stderr, "usage: wol remote set [--db path] <machine> <http|https URL>")
		}
		return 2
	}

	dataStore, device, code := remoteDevice(*databasePath, flags.Arg(0))
	if code != 0 {
		return code
	}
	defer dataStore.Close()
	remoteURL := ""
	if !clear {
		var err error
		remoteURL, err = remoteopen.Validate(flags.Arg(1))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	device.RemoteURL = remoteURL
	if _, err := dataStore.UpdateDevice(context.Background(), device.ID, device); err != nil {
		fmt.Fprintf(os.Stderr, "save remote for %s: %v\n", device.Name, err)
		return 1
	}
	if clear {
		fmt.Printf("cleared remote for %s\n", device.Name)
	} else {
		fmt.Printf("configured remote for %s\n", device.Name)
	}
	return 0
}

func remoteDevice(databasePath, target string) (*store.Store, store.Device, int) {
	dataStore, err := store.Open(databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open local inventory")
		return nil, store.Device{}, 1
	}
	device, err := findStoredDevice(dataStore, target)
	if err != nil {
		dataStore.Close()
		fmt.Fprintln(os.Stderr, err)
		return nil, store.Device{}, 2
	}
	return dataStore, device, 0
}
