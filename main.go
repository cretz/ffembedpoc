package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cretz/ffembedpoc/firefox"
	"github.com/therecipe/qt/widgets"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	app := widgets.NewQApplication(len(os.Args), os.Args)
	window := widgets.NewQMainWindow(nil, 0)
	window.Resize2(800, 800)
	window.SetWindowTitle("FF Embed POC")

	// Build firefox config, could customize logger here
	log, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	config := firefox.Config{
		Log:               log.Sugar(),
		LogRemoteMessages: true,
	}
	// Start firefox
	ff, err := firefox.Start(context.Background(), config)
	if err != nil {
		return err
	}
	defer ff.Close()

	window.SetCentralWidget(ff.Widget)
	window.Show()

	// Exec async
	appDone := make(chan struct{}, 1)
	go func() {
		defer close(appDone)
		app.Exec()
	}()

	// Stop when done or signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-signalCh:
		app.Exit(0)
	case <-appDone:
	}
	return nil
}
