package main

import "github.com/alexzhang1030/time-sync-cli/internal/cmd"

var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
