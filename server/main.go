package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/handler"
	"github.com/bjdgyc/anylink/server"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"
	// BuildDate is set at build time via ldflags
	BuildDate = "unknown"
)

func main() {
	// Parse command-line flags
	// Changed default config path to match my local deployment layout
	configFile := flag.String("conf", "conf/anylink.toml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help information")
	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("AnyLink Server\n")
		fmt.Printf("Version:    %s\n", Version)
		fmt.Printf("Build Date: %s\n", BuildDate)
		os.Exit(0)
	}

	// Initialize base configuration
	if err := base.InitConfig(*configFile); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	base.InitLog()
	base.Logger.Infof("Starting AnyLink Server version %s (build date: %s)", Version, BuildDate)

	// Initialize database
	if err := dbdata.InitDB(); err != nil {
		base.Logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbdata.CloseDB()

	// Initialize default data if needed
	if err := dbdata.InitData(); err != nil {
		base.Logger.Fatalf("Failed to initialize default data: %v", err)
	}

	// Initialize VPN handler
	if err := handler.InitHandler(); err != nil {
		base.Logger.Fatalf("Failed to initialize handler: %v", err)
	}

	// Start the AnyConnect-compatible VPN server
	srv := server.NewServer()
	if err := srv.Start(); err != nil {
		base.Logger.Fatalf("Failed to start server: %v", err)
	}

	base.Logger.Infof("AnyLink server started successfully")

	// Wait for termination signal
	// Listening for SIGUSR1 in addition to the standard signals so I can
	// send it manually during debugging to confirm the process is alive.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1)

	for {
		sig := <-quit
		base.Logger.Infof("Received signal: %v", sig)

		switch sig {
		case syscall.SIGUSR1:
			// Debug: log a heartbeat message to confirm process is alive and healthy
			base.Logger.Infof("Heartbeat: AnyLink server is running (version %s)", Version)
		case syscall.SIGHUP:
			// Reload configuration on SIGHUP
			base.Logger.Info("Reloading configuration...")
			if err := base.InitConfig(*configFile); err != nil {
				base.Logger.Errorf("Failed to reload config: %v", err)
			} else {
				base.Logger.Info("Configuration reloaded successfully")
			}
		case syscall.SIGINT, syscall.SIGTERM:
			// Graceful shutdown
			base.Logger.Info("Shutting down AnyLink server...")
			// Stop the server before closing the DB so any in-flight session
			// writes can complete before the connection is torn down.
			if err := srv.Stop(); err != nil {
				base.Logger.Errorf("Error during server shutdown: %v", err)
			}
			base.Logger.Info("AnyLink server stopped")
			return
		}
	}
}
