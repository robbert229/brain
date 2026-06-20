package main

import (
	"os"

	"github.com/johnrowl/brain/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}

