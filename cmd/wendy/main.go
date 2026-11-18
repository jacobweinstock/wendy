// Command wendy runs Cluster API Provider Tinkerbell end-to-end test platform.
package main

import (
	"os"

	"github.com/tinkerbell/wendy/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
