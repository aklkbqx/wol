package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aklkbqx/wol/internal/api"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wol"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		exitCode := runServer(os.Args[2:])
		os.Exit(exitCode)
	case "wake":
		exitCode := runWake(os.Args[2:])
		os.Exit(exitCode)
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	address := flags.String("addr", "127.0.0.1:8787", "HTTP listen address")
	databasePath := flags.String("db", "wol.db", "SQLite database path")
	webDirectory := flags.String("web-dir", "web/build", "Svelte static assets directory")
	allowedOrigin := flags.String("cors-origin", "http://localhost:5173", "development CORS origin")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if directory := filepath.Dir(*databasePath); directory != "." {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			log.Printf("create database directory: %v", err)
			return 1
		}
	}
	dataStore, err := store.Open(*databasePath)
	if err != nil {
		log.Printf("open database: %v", err)
		return 1
	}
	defer dataStore.Close()

	server := &http.Server{
		Addr:              *address,
		Handler:           api.NewServer(dataStore, *webDirectory, *allowedOrigin).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	shutdownContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("WOL server listening on http://%s", *address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("server: %v", err)
		return 1
	}
	return 0
}

func runWake(arguments []string) int {
	flags := flag.NewFlagSet("wake", flag.ContinueOnError)
	destination := flags.String("destination", "255.255.255.255", "IPv4 destination or broadcast address")
	port := flags.Int("port", 9, "UDP port")
	interfaceName := flags.String("interface", "", "network interface name")
	repeat := flags.Int("repeat", 3, "number of packets")
	interval := flags.Duration("interval", 200*time.Millisecond, "delay between packets")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "wake requires one MAC address")
		return 2
	}
	mac, err := wol.ParseMAC(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ip := net.ParseIP(*destination)
	if ip == nil || ip.To4() == nil {
		fmt.Fprintln(os.Stderr, "destination must be a valid IPv4 address")
		return 2
	}
	result, err := wol.Send(context.Background(), wol.SendRequest{MAC: mac, Destination: ip.To4(), Port: *port, Interface: *interfaceName, Repeat: *repeat, Interval: *interval})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	fmt.Printf("sent %d packets (%d bytes) to %s:%d\n", result.Packets, result.Bytes, result.Destination, result.Port)
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "wol - Wake-on-LAN toolkit")
	fmt.Fprintln(os.Stderr, "usage: wol server [flags] | wol wake [flags] MAC | wol version")
}
