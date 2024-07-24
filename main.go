package main

import (
	"fmt"
	"os"

	drand "github.com/drand/go-clients/internal"
)

func main() {
	app := drand.CLI()
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}
