// Package network provides the TCP transport for the key/value database.
//
// It contains a small server that accepts connections, bounds concurrency
// with a [concurrency.Semaphore], and delegates each request to a
// [TCPHandler] supplied by the caller, plus a matching client that issues
// request/response round-trips.
package network

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/YanBatytskiy/in_memory_base/internal/concurrency"
)

// TCPHandler is the request/response callback invoked for every client
// message. It receives the raw request bytes and must return the response
// bytes; the server handles framing, timeouts and back-pressure.
type TCPHandler = func(context.Context, []byte) []byte

// TCPServer is a line-oriented TCP server with bounded connection
// concurrency. Create one with [NewTCPServer] and drive it with
// [TCPServer.HandleClientQueries].
type TCPServer struct {
	listener  net.Listener
	semaphore *concurrency.Semaphore

	idleTimeout    time.Duration
	maxConnections int
	bufferSize     int

	log *slog.Logger
}

//go:generate go run github.com/vektra/mockery/v3@v3.6.1 --config ../../.mockery.yaml

// NewTCPServer starts listening on address and applies the given options.
// Returns [ErrInvalidMaxConn] or [ErrInvalidBufferSize] if the corresponding
// option was not supplied, and any [net.ListenConfig.Listen] failure
// verbatim. ctx is used to bound the listener setup.
func NewTCPServer(ctx context.Context, address string, log *slog.Logger, options ...TCPServeOption) (*TCPServer, error) {
	// const op = "network.NewTCPServer"

	if log == nil {
		return nil, ErrInvalidLogger
	}

	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	server := &TCPServer{
		listener: listener,
		log:      log,
	}

	for _, option := range options {
		option(server)
	}

	if server.maxConnections == 0 {
		return nil, ErrInvalidMaxConn
	}

	if server.bufferSize == 0 {
		return nil, ErrInvalidBufferSize
	}

	server.semaphore = concurrency.NewSemaphore(server.maxConnections)

	return server, nil
}

// HandleClientQueries accepts connections in a background goroutine and
// dispatches each one to [TCPServer.HandleConnections] while respecting the
// max-connections semaphore. It returns when ctx is cancelled and the
// listener is closed.
func (server *TCPServer) HandleClientQueries(ctx context.Context, handler TCPHandler) {
	const op = "network.HandleClientQueries"
	server.log.Info("server listening", slog.String("address", server.listener.Addr().String()))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			connection, errorReturn := server.listener.Accept()
			if errorReturn != nil {
				if errors.Is(errorReturn, net.ErrClosed) {
					errorReturn = nil
					return
				}
				server.log.Debug("failed to accept connection",
					slog.String("operation", op),
					slog.String("error",
						errorReturn.Error()))
				continue
			}

			server.semaphore.Acquire()
			go func(connection net.Conn) {
				defer server.semaphore.Release()
				server.HandleConnections(ctx, connection, handler)
			}(connection)
		}
	}()

	<-ctx.Done()
	err := server.listener.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		server.log.Debug("failed to close listener on shutdown",
			slog.String("error", err.Error()))
	}
	wg.Wait()
}

// HandleConnections drives a single TCP connection: it loops reading up to
// bufferSize bytes, passes them to handler, writes the response back, and
// applies the idle timeout to both sides. Any panic in handler is
// recovered and logged so one bad request does not take the server down.
//
//nolint:gocognit // per-connection loop handles framing, timeouts, and handler errors in-place
func (server *TCPServer) HandleConnections(ctx context.Context, connection net.Conn, handler TCPHandler) {
	const op = "network.HandleConnections"

	defer func() {
		if r := recover(); r != nil {
			server.log.Error("panic in connection handler",
				slog.String("operation", op),
				slog.Any("panic", r))
		}

		err := connection.Close()
		if err != nil {
			server.log.Debug("failed to accept connection",
				slog.String("operation", op),
				slog.String("error",
					err.Error()))
		}
	}()

	for {
		if server.idleTimeout != 0 {
			err := connection.SetReadDeadline(time.Now().Add(server.idleTimeout))
			if err != nil {
				server.log.Debug("failed to set read deadline", slog.String("operation", op))
				break
			}
		}

		query := make([]byte, server.bufferSize)

		num, err := connection.Read(query)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				server.log.Debug("failed to read from connection",
					slog.String("operation", op),
					slog.String("error", err.Error()))
			}
			break
		}

		response := handler(ctx, query[:num])

		if server.idleTimeout != 0 {
			err = connection.SetWriteDeadline(time.Now().Add(server.idleTimeout))
			if err != nil {
				server.log.Debug("failed to set write deadline", slog.String("operation", op))
				break
			}
		}
		_, err = connection.Write(response)
		if err != nil {
			break
		}
	}
}
