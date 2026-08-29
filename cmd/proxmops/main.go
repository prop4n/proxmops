package main

import (
	"os"

	"github.com/prop4n/proxmops/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
