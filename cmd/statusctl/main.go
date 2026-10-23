// Command statusctl posts incidents and maintenance to a status page
// repository and renders it as a static site.
package main

import (
	"os"
	_ "time/tzdata" // timezone data for hosts without a zoneinfo database (Windows, scratch images)

	"github.com/Fab-IT-ApS/statusctl/internal/cli"
)

func main() { os.Exit(cli.Main()) }
