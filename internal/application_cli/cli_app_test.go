package applicationcli

import (
	"bytes"
	"flag"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateLoggerLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level string
	}{
		{
			name:  "info",
			level: LoggerLevelInfo,
		},
		{
			name:  "dev",
			level: LoggerLevelDev,
		},
		{
			name:  "prod",
			level: LoggerLevelProd,
		},
		{
			name:  "unknown defaults",
			level: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log, err := CreateLogger(tc.level)
			require.NoError(t, err)
			require.NotNil(t, log)
		})
	}
}

func TestAppCliRun_Table(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContain []string
	}{
		{
			name:        "exit after command",
			input:       "PING\nexit\n",
			wantContain: []string{"RESP:PING", "Input command"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil && strings.Contains(err.Error(), "operation not permitted") {
				t.Skip("net.Listen not permitted in this environment")
			}
			require.NoError(t, err)
			defer ln.Close()

			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				buf := make([]byte, 32)
				n, _ := conn.Read(buf)
				_, _ = conn.Write([]byte("RESP:" + string(bytes.TrimSpace(buf[:n]))))
			}()

			oldStdin := os.Stdin
			oldStdout := os.Stdout

			stdinR, stdinW, _ := os.Pipe()
			_, _ = stdinW.WriteString(tc.input)
			_ = stdinW.Close()

			stdoutR, stdoutW, _ := os.Pipe()

			os.Stdin = stdinR
			os.Stdout = stdoutW

			t.Cleanup(func() {
				os.Stdin = oldStdin
				os.Stdout = oldStdout
			})

			oldArgs := os.Args
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			os.Args = []string{
				"cli",
				"--",
				"-address", ln.Addr().String(),
				"-idle", "100ms",
				"-mes_size", "16",
				"-debug=false",
			}
			t.Cleanup(func() { os.Args = oldArgs })

			app := NewAppCli()
			app.Run(t.Context())

			_ = stdoutW.Close()
			outBytes, _ := io.ReadAll(stdoutR)

			<-serverDone

			out := string(outBytes)
			for _, want := range tc.wantContain {
				require.Contains(t, out, want)
			}
		})
	}
}
