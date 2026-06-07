package main

import (
	"fmt"
	"os"

	"github.com/coffee-code-io/coffeeenv/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "coffeeenv: "+err.Error())
		os.Exit(1)
	}
}
