package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aklkbqx/wol/internal/doctor"
	config "github.com/aklkbqx/wol/internal/networkconfig"
)

func runDoctor(args []string) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	envFile := flags.String("env-file", envString("WOL_ENV_FILE", ".wol.env"), "optional network settings file")
	if err := flags.Parse(args); err != nil {
		return 2
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
		fmt.Fprintf(os.Stderr, "load environment configuration: %v\n", err)
		return 1
	}
	report := doctor.RunDoctorWithEnv(cwd, defaults)
	fmt.Println(report.RenderReport())

	hasFail := false
	for _, it := range report.Items {
		if it.Status == "FAIL" {
			hasFail = true
			break
		}
	}

	if hasFail {
		return 1
	}
	return 0
}
