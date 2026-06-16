package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	rpcStdout := os.Stdout
	log.SetFlags(0)
	log.SetPrefix("[go-deep] ")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		os.Exit(0)
	}()

	if len(os.Args) < 2 {
		log.Fatal("usage: go-deep serve [flags]")
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(ctx, rpcStdout, os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}
