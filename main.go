package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/hadi77ir/go-logging"
	"github.com/hadi77ir/nativemessagingproxy/pkg/log"
	"os"

	runnable "github.com/hadi77ir/go-runnable"
	"github.com/hadi77ir/nativemessagingproxy/pkg/config"
	"github.com/hadi77ir/nativemessagingproxy/pkg/server"
)

func main() {
	helpFlag := flag.Bool("help", false, "Show help")
	flag.Parse()
	if *helpFlag {
		fmt.Println("usage: nativemessagingproxy [-help]")
		fmt.Println("config: ", config.ConfigPath())
		fmt.Println("config path can be set through NMPROXY_CONFIG.")
		fmt.Println("for more info on usage and configuration, take a look at the documentation.")
		return
	}
	cfg := config.FailsafeReadConfig()
	if err := log.InitLogger(cfg.LogPath); err != nil {
		panic(err)
	}
	logger := log.Global()

	if cfg.Command == "" {
		logger.Log(logging.FatalLevel, "command needs to be specified")
		os.Exit(1)
		return
	}
	logger.Log(logging.InfoLevel, "running bridge")
	err := runnable.Run(server.RunBridge, cfg, logger, context.Background())
	if err != nil {
		logger.Log(logging.ErrorLevel, "error: ", err)
	}
}
