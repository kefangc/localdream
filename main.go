package main

import (
	"log"
	"os"

	"localdream/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
