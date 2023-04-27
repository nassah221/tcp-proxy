package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
)

var (
	cfgPath = flag.String("config", "../config.json", "path to config file")
)

func main() {
	flag.Parse()
	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatal("failed to load config: ", err)
	}

	fmt.Println(cfg, *cfgPath)

	apps := NewApps(cfg)
	srv := NewServer(apps)

	ctx := context.Background()
	sigCtx, _ := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)

	if err := srv.Start(sigCtx); err != nil {
		log.Println(err)
	}

	log.Println("proxy shutdown")
}
