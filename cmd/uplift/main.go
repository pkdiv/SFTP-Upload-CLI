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

	app := cli.New(path, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(app.Run(flag.Args()))
}


