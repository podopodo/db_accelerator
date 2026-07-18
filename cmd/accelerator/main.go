package main

import (
	"os"

	"github.com/podopodo/db_accelerator/internal/command"
)

func main() {
	os.Exit(command.Run(os.Args[1:], os.Stdout, os.Stderr))
}
