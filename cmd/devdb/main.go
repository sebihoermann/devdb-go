package main

import (
	"os"

	"github.com/sebihoermann/devdb-go/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
