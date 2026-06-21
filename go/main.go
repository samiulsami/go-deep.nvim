package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/samiulsami/go-deep.nvim/go/index"
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
	case "build-index":
		err = runBuildIndex(ctx, os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runBuildIndex(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("build-index", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexFilePath string
	fs.StringVar(&indexFilePath, "index-file-path", "", "path to persistent stdlib symbol index file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("build-index: unexpected args: %v", fs.Args())
	}
	if indexFilePath == "" {
		var err error
		indexFilePath, err = index.DefaultCachePath()
		if err != nil {
			return err
		}
	}
	return index.BuildAndSave(ctx, indexFilePath)
}
