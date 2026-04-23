// Command cli is an interactive REPL client for the in-memory KV server.
//
// It connects to a running server over TCP and forwards each line typed on
// stdin as a text command. See [internal/application_cli] for the available
// flags (-address, -idle, -mes_size, -debug).
package main

import (
	"context"
	"os/signal"
	"syscall"

	applicationcli "github.com/YanBatytskiy/in_memory_base/internal/application_cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	appCli := applicationcli.NewAppCli()
	appCli.Run(ctx)
}
