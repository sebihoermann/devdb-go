package main

import (
	"os"

	openclaw "github.com/sebihoermann/devdb-go/internal/integrations/openclaw"
)

func main() {
	os.Exit(openclaw.Execute())
}
