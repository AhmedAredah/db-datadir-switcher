package main

import (
	"fmt"
	"os"

	"mariadb-monitor/cli"
	"mariadb-monitor/core"
)

func main() {
	// Initialize core
	core.Init()
	core.InitCredentials()

	// Parse command line arguments
	if len(os.Args) < 2 {
		// Default to GUI mode
		fmt.Println("No command specified. Use 'help' for usage information or run GUI mode.")
		fmt.Println("For now, running CLI help:")
		cli := cli.NewCLI()
		cli.ShowHelp()
		return
	}

	command := os.Args[1]
	cli := cli.NewCLI()

	switch command {
	case "list":
		if err := cli.List(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "status":
		if err := cli.Status(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		if len(os.Args) < 3 {
			fmt.Println("Error: Configuration name required")
			fmt.Println("Usage: dbswitcher start <config-name>")
			os.Exit(1)
		}
		if err := cli.Start(os.Args[2]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "switch":
		if len(os.Args) < 3 {
			fmt.Println("Error: Configuration name required")
			fmt.Println("Usage: dbswitcher switch <config-name>")
			os.Exit(1)
		}
		if err := cli.Switch(os.Args[2]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "stop":
		if err := cli.Stop(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "gui":
		fmt.Println("GUI mode not yet implemented in refactored version")
		fmt.Println("GUI will be available in the next phase of refactoring")

	case "tray":
		fmt.Println("Tray mode not yet implemented in refactored version")
		fmt.Println("Tray will be available in the next phase of refactoring")

	case "help", "--help", "-h":
		cli.ShowHelp()

	case "version", "--version", "-v":
		fmt.Println("DBSwitcher v3.0.0 - MariaDB Configuration Manager (Refactored)")

	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Use 'help' for usage information")
		os.Exit(1)
	}
}