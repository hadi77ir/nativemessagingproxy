package nativemessagingproxy

import (
	"context"
	"os"

	runnable "github.com/hadi77ir/go-runnable"
	"github.com/hadi77ir/nativemessagingproxy/pkg/config"
	"github.com/hadi77ir/nativemessagingproxy/pkg/server"
)

func main() {
	_, _ = os.Stderr.WriteString("config: ")
	_, _ = os.Stderr.WriteString(config.ConfigPath())
	_, _ = os.Stderr.WriteString("\n")
	cfg := config.FailsafeReadConfig()

	if cfg.Command == "" {
		_, _ = os.Stderr.WriteString("command needs to be specified\n")
		os.Exit(1)
		return
	}
	_, _ = os.Stderr.WriteString("running bridge\n\n\n\n")
	err := runnable.Run(server.RunBridge, cfg, nil, context.Background())
	if err != nil {
		_, _ = os.Stderr.WriteString("\n\n\nerror: " + err.Error())
	}
}
