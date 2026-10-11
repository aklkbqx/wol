package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aklkbqx/wol/internal/releasesign"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "sign" && args[0] != "verify") {
		return fmt.Errorf("usage: wol-release-sign sign|verify --key PEM --manifest SHA256SUMS --output FILE|--signature FILE")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	keyPath := flags.String("key", "", "Ed25519 PEM key file")
	manifestPath := flags.String("manifest", "", "exact checksum manifest file")
	var destination *string
	if args[0] == "sign" {
		destination = flags.String("output", "", "new detached signature file")
	} else {
		destination = flags.String("signature", "", "detached signature file")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *keyPath == "" || *manifestPath == "" || *destination == "" {
		return fmt.Errorf("key, manifest and output/signature paths are required")
	}
	keyBytes, err := releasesign.ReadFile(*keyPath, releasesign.MaxKeyBytes)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	manifest, err := releasesign.ReadFile(*manifestPath, releasesign.MaxManifestBytes)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if args[0] == "verify" {
		key, err := releasesign.ParsePublicKey(keyBytes)
		if err != nil {
			return err
		}
		signature, err := releasesign.ReadFile(*destination, ed25519.SignatureSize)
		if err != nil {
			return fmt.Errorf("read signature: %w", err)
		}
		if err := releasesign.Verify(key, manifest, signature); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "Manifest signature verified.")
		return err
	}
	key, err := releasesign.ParsePrivateKey(keyBytes)
	if err != nil {
		return err
	}
	signature, err := releasesign.Sign(key, manifest)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create signature (existing files are never overwritten): %w", err)
	}
	_, writeErr := f.Write(signature)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(*destination)
		return fmt.Errorf("write signature: %w", writeErr)
	}
	_, err = fmt.Fprintln(out, "Detached manifest signature written.")
	return err
}
