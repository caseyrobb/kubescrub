package main

import (
	"os"

	"github.com/caseyrobb/kubescrub/internal/app"
)

func main() {
	if err := app.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
