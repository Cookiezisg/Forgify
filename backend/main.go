package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sunweilin/forgify/internal/server"
	"github.com/sunweilin/forgify/internal/storage"
)

func main() {
	dataDir := storage.DefaultDataDir()
	if err := storage.Init(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "storage init failed: %v\n", err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen failed: %v\n", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Print port to stdout so Electron can read it
	fmt.Printf("FORGIFY_PORT=%d\n", port)
	os.Stdout.Sync()

	srv := server.New()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		<-sig
		listener.Close()
		os.Exit(0)
	}()

	if err := http.Serve(listener, srv); err != nil {
		os.Exit(0) // listener closed on shutdown
	}
}
