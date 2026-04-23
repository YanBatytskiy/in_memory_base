package network

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// TCPClient is a minimal synchronous request/response client for the TCP
// server. One goroutine at a time may call [TCPClient.SendAndReceive]; the
// call writes a request, waits for the reply, and returns it with trailing
// whitespace trimmed.
type TCPClient struct {
	conn net.Conn

	address        string
	idleTimeout    time.Duration
	maxMessageSize int

	log *slog.Logger
}

// NewTCPClient dials address (configured via [WithClientTCPAddress]) and
// returns a ready client. The connection is established immediately so
// errors surface at construction time. The dial is bounded by the caller's
// ctx; if the client was configured with an idle timeout via
// [WithClientTCPIdleTimeout], it also limits the dial duration.
func NewTCPClient(
	ctx context.Context,
	log *slog.Logger,
	options ...TCPClientOption,
) (*TCPClient, error) {
	const op = "network.NewTCPClient"

	if log == nil {
		return nil, fmt.Errorf("%s: failed to initialize logger", op)
	}

	client := &TCPClient{
		log: log,
	}

	for _, option := range options {
		option(client)
	}

	dialCtx := ctx
	if client.idleTimeout > 0 {
		var cancel context.CancelFunc
		dialCtx, cancel = context.WithTimeout(ctx, client.idleTimeout)
		defer cancel()
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(dialCtx, "tcp", client.address)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to connect to server: %w", op, err)
	}

	client.conn = conn

	return client, nil
}

// SendAndReceive writes request to the server and waits for a single
// response, both under the configured idle timeout. The returned slice has
// leading/trailing whitespace trimmed.
func (tcpClient *TCPClient) SendAndReceive(request []byte) ([]byte, error) {
	const op = "network.TCPClient.SendAndReceive"

	var err error

	if tcpClient.idleTimeout != 0 {
		err = tcpClient.conn.SetWriteDeadline(time.Now().Add(tcpClient.idleTimeout))
		if err != nil {
			tcpClient.log.Debug("failed to set write deadline", slog.String("operation", op))
		}
	}
	_, err = tcpClient.conn.Write(request)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to send data to server %w", op, err)
	}

	response := make([]byte, tcpClient.maxMessageSize)

	if tcpClient.idleTimeout != 0 {
		err = tcpClient.conn.SetReadDeadline(time.Now().Add(tcpClient.idleTimeout))
		if err != nil {
			tcpClient.log.Debug("failed to set read deadline", slog.String("operation", op))
		}
	}

	num, err := tcpClient.conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read data from server %w", op, err)
	}

	return bytes.TrimSpace(response[:num]), nil
}

// Close releases the underlying connection. Safe to call on an
// already-closed client.
func (tcpClient *TCPClient) Close() {
	if tcpClient.conn != nil {
		err := tcpClient.conn.Close()
		if err != nil {
			tcpClient.log.Debug("failed to close tcp client connection",
				slog.String("error", err.Error()))
		}
	}
}
