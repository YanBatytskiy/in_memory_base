package network

import "time"

// TCPClientOption configures a [TCPClient] via the functional-options
// pattern. Pass zero or more of these to [NewTCPClient].
type TCPClientOption func(*TCPClient)

// TCPServeOption configures a [TCPServer] via the functional-options
// pattern. Pass zero or more of these to [NewTCPServer].
type TCPServeOption func(*TCPServer)

// WithClientTCPAddress sets the server address the client should dial.
func WithClientTCPAddress(address string) TCPClientOption {
	return func(client *TCPClient) {
		client.address = address
	}
}

// WithClientTCPIdleTimeout sets the per-operation read/write deadline on
// the client. Negative values fall back to 5 minutes.
func WithClientTCPIdleTimeout(timeout time.Duration) TCPClientOption {
	return func(client *TCPClient) {
		if timeout < 0 {
			timeout = 5 * time.Minute
		}
		client.idleTimeout = timeout
	}
}

// WithClientTCPMaxMessageSize sets the client-side receive buffer (the
// largest response the client is willing to read in one call). Negative
// values fall back to 4098 bytes.
func WithClientTCPMaxMessageSize(maxMessageSize int) TCPClientOption {
	return func(client *TCPClient) {
		if maxMessageSize < 0 {
			maxMessageSize = 4098
		}
		client.maxMessageSize = maxMessageSize
	}
}

// WithServerTCPIdleTimeout sets the per-operation read/write deadline on
// server connections. Negative values fall back to 5 minutes.
func WithServerTCPIdleTimeout(timeout time.Duration) TCPServeOption {
	return func(server *TCPServer) {
		if timeout < 0 {
			timeout = 5 * time.Minute
		}
		server.idleTimeout = timeout
	}
}

// WithServerTCPMaxConnectionNumber caps the number of connections served
// concurrently. The server uses a [concurrency.Semaphore] to block Accept
// once the limit is reached. Negative values fall back to 100.
func WithServerTCPMaxConnectionNumber(maxConnections int) TCPServeOption {
	return func(server *TCPServer) {
		if maxConnections < 0 {
			maxConnections = 100
		}
		server.maxConnections = maxConnections
	}
}

// WithServerTCPBufferSize sets the server-side receive buffer (the largest
// single request the server will read). Negative values fall back to 4098
// bytes.
func WithServerTCPBufferSize(bufferSize int) TCPServeOption {
	return func(server *TCPServer) {
		if bufferSize < 0 {
			bufferSize = 4098
		}
		server.bufferSize = bufferSize
	}
}
