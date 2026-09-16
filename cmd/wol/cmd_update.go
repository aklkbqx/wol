package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/buildinfo"
	"github.com/aklkbqx/wol/internal/updater"
)

func runUpdate(arguments []string) int {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	checkOnly := flags.Bool("check", false, "check for available updates without installing")
	flags.BoolVar(checkOnly, "c", false, "check for available updates without installing (shorthand)")
	force := flags.Bool("force", false, "force reinstallation even if already on latest version")
	flags.BoolVar(force, "f", false, "force reinstallation (shorthand)")
	targetVersion := flags.String("version", "", "install a specific release version tag (e.g. v0.4.7)")
	repo := flags.String("repo", envString("WOL_UPDATE_REPO", updater.DefaultRepo), "GitHub repository in owner/repo format")
	targetFile := flags.String("target", "", "target executable path (defaults to current binary)")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum time allowed for update operation")

	if err := flags.Parse(arguments); err != nil {
		return 2
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument: %s\nusage: wol update [--check] [--force] [--version TAG] [--target PATH]\n", flags.Arg(0))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := updater.NewClient(buildinfo.Version, updater.WithRepo(*repo))

	currentTag := buildinfo.Version
	if !strings.HasPrefix(currentTag, "v") && !strings.HasPrefix(currentTag, "V") {
		currentTag = "v" + currentTag
	}

	if *checkOnly {
		fmt.Printf("Checking for updates (current: %s)...\n", currentTag)
	} else if strings.TrimSpace(*targetVersion) != "" {
		fmt.Printf("Fetching release %s (current: %s)...\n", *targetVersion, currentTag)
	} else {
		fmt.Printf("Checking for updates (current: %s)...\n", currentTag)
	}

	result, err := client.ExecuteUpdate(ctx, updater.UpdateOptions{
		CheckOnly:     *checkOnly,
		Force:         *force,
		TargetVersion: *targetVersion,
		TargetFile:    *targetFile,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "update error: %v\n", err)
		return 1
	}

	if result.CheckOnly {
		if result.UpToDate {
			fmt.Printf("WOL is up to date (%s).\n", currentTag)
			return 0
		}
		fmt.Printf("Update available: %s -> %s\n", currentTag, result.NewVersion)
		if result.Release != nil && result.Release.HTMLURL != "" {
			fmt.Printf("Release details: %s\n", result.Release.HTMLURL)
		}
		fmt.Println("Run 'wol update' to install the update.")
		return 0
	}

	if result.UpToDate {
		fmt.Printf("WOL is already up to date (%s). Use --force to reinstall.\n", currentTag)
		return 0
	}

	fmt.Printf("Successfully updated WOL to %s (%s)\n", result.NewVersion, result.InstalledPath)
	return 0
}
