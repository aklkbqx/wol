package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
)

func runStatus(arguments []string) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "inventory database path")
	timeout := flags.Duration("timeout", 3*time.Second, "maximum probe time")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "usage: wol status [--timeout 3s] <machine-name-or-id>")
		return 2
	}
	repository, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open local inventory")
		return 1
	}
	defer repository.Close()
	device, err := findStoredDevice(repository, flags.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if strings.TrimSpace(device.IPAddress) == "" {
		fmt.Printf("%s  UNKNOWN  no IP address configured\n", device.Name)
		return 1
	}
	port := device.VerifyPort
	if port == 0 {
		port = 3389
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result := presence.NewDetector().Probe(ctx, presence.Target{DeviceID: device.ID, IPAddress: device.IPAddress, VerifyPort: port}, *timeout)
	fmt.Printf("%s  %s  via %s  %dms\n", device.Name, strings.ToUpper(string(result.Status)), result.Method, result.LatencyMS)
	if result.Status == presence.StatusOnline {
		return 0
	}
	return 1
}

func runExport(arguments []string) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "inventory database path")
	output := flags.String("output", "", "output file (stdout when omitted)")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: wol export [--output inventory.json]")
		return 2
	}
	repository, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open local inventory")
		return 1
	}
	defer repository.Close()
	data, err := repository.Export(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "export inventory: %v\n", err)
		return 1
	}
	encoded, err := store.EncodeExport(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode inventory: %v\n", err)
		return 1
	}
	if *output == "" {
		fmt.Println(string(encoded))
		return 0
	}
	if err := os.WriteFile(*output, append(encoded, '\n'), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write export: %v\n", err)
		return 1
	}
	fmt.Printf("Exported %d machines and %d routes.\n", len(data.Devices), len(data.WakeRelays))
	return 0
}

func runImport(arguments []string) int {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	databasePath := flags.String("db", envString("WOL_DB", store.DefaultDatabasePath()), "inventory database path")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wol import <inventory.json>")
		return 2
	}
	input, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open import: %v\n", err)
		return 1
	}
	defer input.Close()
	decoder := json.NewDecoder(io.LimitReader(input, 4<<20))
	decoder.DisallowUnknownFields()
	var data store.ExportData
	if err := decoder.Decode(&data); err != nil {
		fmt.Fprintf(os.Stderr, "decode import: %v\n", err)
		return 1
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		fmt.Fprintln(os.Stderr, "decode import: multiple JSON values are not allowed")
		return 1
	}
	repository, err := store.Open(*databasePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not open local inventory")
		return 1
	}
	defer repository.Close()
	if err := repository.Import(context.Background(), data); err != nil {
		fmt.Fprintf(os.Stderr, "import inventory: %v\n", err)
		return 1
	}
	fmt.Printf("Imported %d machines and %d routes.\n", len(data.Devices), len(data.WakeRelays))
	return 0
}
