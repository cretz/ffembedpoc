package main

import (
	"context"
	"log"

	"github.com/cretz/ffembedpoc/firefox"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Start firefox
	ff, err := firefox.Start(context.Background(), firefox.Config{})
	if err != nil {
		return err
	}
	defer ff.Close()
	// // Get window ID
	// winID, err := ff.GetWindowID(context.Background())
	// fmt.Printf("Got %v - %v\n", winID, err)
	return nil
}
