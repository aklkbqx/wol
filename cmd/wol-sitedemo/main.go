// Command wol-sitedemo records the Wake Desk for the website: it walks the
// real terminal UI through its keys on a sample inventory and writes every
// screen as JSON. Nothing is sent on the network.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// output is the file the website loads: one demo per terminal size.
type output struct {
	Keys  []string    `json:"keys"`
	Demos []*tui.Demo `json:"demos"`
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("wol-sitedemo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("out", "", "JSON file to write (default: standard output)")
	sizes := flags.String("sizes", "100x28,40x36", "comma-separated terminal sizes, COLSxROWS")
	maxStates := flags.Int("max", 6000, "most screens per size")
	version := flags.String("version", strings.TrimSuffix(buildinfo.Version, "-dev"), "version shown in the header")
	timeout := flags.Duration("timeout", 20*time.Minute, "give up after this long")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("usage: wol-sitedemo [--out FILE] [--sizes 100x28,40x36] [--max N]: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("usage: wol-sitedemo [--out FILE] [--sizes 100x28,40x36] [--max N]")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	result := output{Keys: tui.DemoKeys}
	for _, size := range strings.Split(*sizes, ",") {
		w, h, err := parseSize(size)
		if err != nil {
			return err
		}
		demo, err := tui.BuildDemo(ctx, tui.DemoOptions{Width: w, Height: h, MaxStates: *maxStates, Version: *version})
		if err != nil {
			return fmt.Errorf("demo %dx%d: %w", w, h, err)
		}
		result.Demos = append(result.Demos, demo)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if *path == "" {
		_, err = out.Write(append(data, '\n'))
		return err
	}
	if err := os.WriteFile(*path, data, 0o644); err != nil {
		return fmt.Errorf("write demo: %w", err)
	}
	for _, d := range result.Demos {
		fmt.Fprintf(out, "%dx%d: %d screens, %d rows\n", d.Width, d.Height, len(d.States), len(d.Rows))
	}
	fmt.Fprintf(out, "wrote %s (%d KB)\n", *path, len(data)/1024)
	return nil
}

func parseSize(value string) (int, int, error) {
	cols, rows, ok := strings.Cut(strings.TrimSpace(value), "x")
	w, errW := strconv.Atoi(cols)
	h, errH := strconv.Atoi(rows)
	if !ok || errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("size %q: want COLSxROWS, for example 100x28", value)
	}
	return w, h, nil
}
