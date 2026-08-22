package main

import (
	"context"
	"os"

	"github.com/gonfff/jotmd/internal/app"
)

var version string

func main() {
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version))
}
