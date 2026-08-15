package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pkdiv/uplift/internal/cli"
	"github.com/pkdiv/uplift/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to configuration file")
	homeDir := flag.Bool("home", false, "open TUI in user home directory")
	filesList := flag.String("files-list", "", "path to a text file containing relative file paths, one per line")
	flag.Parse()

	path := *configPath
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	app := cli.NewWithHome(path, os.Stdin, os.Stdout, os.Stderr, *homeDir)
	app.FilesList = *filesList
	os.Exit(app.Run(flag.Args()))
}


