package main

import (
	"os"

	"github.com/robbert229/brain/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
