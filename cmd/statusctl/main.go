// Command statusctl posts incidents and maintenance to a status page
// repository and renders it as a static site.
package main

import (
	"os"
	_ "time/tzdata" // timezone data for hosts without a zoneinfo database (Windows, scratch images)

	"statusctl/internal/cli"
)

func main() { os.Exit(cli.Main()) }
