// Package applicationcli implements the interactive REPL of the cli binary.
//
// It parses command-line flags, builds a [network.TCPClient], and loops on
// stdin forwarding each entered line to the server until the user types
// "exit" or closes stdin.
package applicationcli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogdiscard"
	"github.com/YanBatytskiy/in_memory_base/internal/lib/logger/slogpretty"
	"github.com/YanBatytskiy/in_memory_base/internal/network"
)

// AppCli is the interactive REPL. It holds no state; the type exists so
// the cli binary can construct and drive it through the usual New/Run
// pattern.
type AppCli struct{}

// NewAppCli returns a new AppCli.
func NewAppCli() *AppCli {
	return &AppCli{}
}

// Run parses flags, connects to the server, and enters the REPL loop.
// It returns when the user types "exit", closes stdin, ctx is cancelled,
// or when a non-recoverable error occurs (in which case the process exits
// via [os.Exit]). ctx is used to bound the TCP dial.
func (appCli *AppCli) Run(ctx context.Context) {
	const op = "AppCli.Run"

	address, idleTimeout, maxMessageSize, env := appCli.parseFlags()

	log, err := CreateLogger(env)
	if err != nil {
		_ = log
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	tcpClient := appCli.buildClient(ctx, log, address, idleTimeout, maxMessageSize)
	appCli.replLoop(log, tcpClient, op)
}

// parseFlags reads CLI flags and returns the connection settings along with
// the logger environment string.
func (appCli *AppCli) parseFlags() (address string, idleTimeout time.Duration, maxMessageSize int, env string) {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}

	addressFlag := flag.String("address", "127.0.0.1:3323", "Address of server")
	idleFlag := flag.Duration("idle", time.Minute*5, "Idle timeout of connections")
	maxMessageSizeFlag := flag.Int("mes_size", 4096, "Max message size")
	debug := flag.Bool("debug", true, "Debug environment")

	flag.Parse()

	env = "info"
	if *debug {
		env = "dev"
	}

	address = *addressFlag
	if address == "" {
		address = "127.0.0.1:3323"
	}

	maxMessageSize = *maxMessageSizeFlag
	if maxMessageSize <= 0 {
		maxMessageSize = 4096
	}

	idleTimeout = *idleFlag
	return address, idleTimeout, maxMessageSize, env
}

// buildClient constructs a [network.TCPClient] with the given parameters.
// ctx bounds the underlying TCP dial. On a connection error the process
// exits via [os.Exit].
func (appCli *AppCli) buildClient(ctx context.Context, log *slog.Logger, address string, idleTimeout time.Duration, maxMessageSize int) *network.TCPClient {
	//nolint:prealloc // three fixed appends below
	var options []network.TCPClientOption

	options = append(options, network.WithClientTCPAddress(address))
	options = append(options, network.WithClientTCPMaxMessageSize(maxMessageSize))
	options = append(options, network.WithClientTCPIdleTimeout(idleTimeout))

	tcpClient, err := network.NewTCPClient(ctx, log, options...)
	if err != nil {
		log.Info("cannot connect to server", slog.String("address", address), slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("connected to server", slog.String("address", address))
	return tcpClient
}

// replLoop is the interactive read-eval-print loop: reads a line from stdin,
// forwards it to the server, prints the response, and exits when the user
// types "exit" or stdin is closed.
func (appCli *AppCli) replLoop(log *slog.Logger, tcpClient *network.TCPClient, op string) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Fprintln(os.Stdout, "\nInput command (exit for exit)")
		fmt.Fprint(os.Stdout, "> ")

		request, err := reader.ReadString('\n')
		if err != nil {
			log.Info("input closed, exiting")
			return
		}

		request = strings.TrimSpace(request)

		if request == "" {
			fmt.Fprint(os.Stdout, "> ")
			continue
		}

		if strings.EqualFold(request, "exit") {
			log.Info("Cli exit and finished", slog.String("operation", op))
			return
		}

		response, err := tcpClient.SendAndReceive([]byte(request))
		if err != nil {
			log.Info("failed to send command", slog.String("error", err.Error()))
			fmt.Fprint(os.Stdout, "> ")
			continue
		}

		fmt.Fprintln(os.Stdout, string(response))
		fmt.Fprint(os.Stdout, "> ")
	}
}

// Supported logger-level strings for [CreateLogger].
const (
	// LoggerLevelInfo enables Info+ logs via the pretty handler.
	LoggerLevelInfo = "info"
	// LoggerLevelDev enables Debug+ logs via the pretty handler.
	LoggerLevelDev = "dev"
	// LoggerLevelProd silences all logs.
	LoggerLevelProd = "prod"
)

// CreateLogger returns a [slog.Logger] matching env; unknown values fall
// back to Info.
func CreateLogger(env string) (*slog.Logger, error) {
	var log *slog.Logger

	switch env {
	case LoggerLevelInfo:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelInfo},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)

	case LoggerLevelDev:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelDebug},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)
	case LoggerLevelProd:
		log = slogdiscard.NewDiscardLogger()
	default:
		opts := slogpretty.PrettyHandlerOptions{
			SlogOpts: &slog.HandlerOptions{Level: slog.LevelInfo},
		}
		handler := opts.NewPrettyHandler(os.Stdout)

		log = slog.New(handler)

	}
	log.Info("starting service", slog.String("logger level", env))
	return log, nil
}
