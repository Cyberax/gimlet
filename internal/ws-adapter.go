package internal

import (
	"context"
	"crypto/tls"
	"golang.org/x/net/websocket"
	"net"
	"time"
)

type GoWSAdapter struct {
	conn *websocket.Conn
}

var _ WsConn = &GoWSAdapter{}

func NewWSAdapter(conn *websocket.Conn) *GoWSAdapter {
	return &GoWSAdapter{
		conn: conn,
	}
}

func (g *GoWSAdapter) SetReadDeadline(t time.Time) error {
	return g.conn.SetDeadline(t)
}

func (g *GoWSAdapter) SetWriteDeadline(t time.Time) error {
	return g.conn.SetWriteDeadline(t)
}

func (g *GoWSAdapter) ReadMessage() ([]byte, error) {
	var data []byte
	err := websocket.Message.Receive(g.conn, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (g *GoWSAdapter) WriteMessage(data []byte) error {
	return websocket.Message.Send(g.conn, data)
}

func (g *GoWSAdapter) WriteText(data string) error {
	return websocket.Message.Send(g.conn, data)
}

func (g *GoWSAdapter) SendPing() error {
	w, err := g.conn.NewFrameWriter(websocket.PingFrame)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("keepalive"))
	if err != nil {
		return err
	}
	return w.Close()
}

func DialWebSocket(ctx context.Context, config *websocket.Config) (*websocket.Conn, error) {
	conn, err := connectToSocket(ctx, config)
	if err != nil {
		return nil, err
	}

	// Make sure we clean up the connection on any failure
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	// The websocket.NewClient() function can block indefinitely, make sure that we
	// respect the deadlines specified by the context.
	doneChan := make(chan bool)
	defer close(doneChan)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now()) // Force the pending operations to fail
		case <-doneChan:
			return
		}
	}()

	// Create a new websocket and do the handshake
	ws, err := websocket.NewClient(config, conn)
	if err != nil {
		return nil, err
	}

	success = true
	return ws, nil
}

func connectToSocket(ctx context.Context, config *websocket.Config) (net.Conn, error) {
	location := config.Location

	if location == nil {
		return nil, websocket.ErrBadWebSocketLocation
	}
	if config.Origin == nil {
		return nil, websocket.ErrBadWebSocketOrigin
	}

	dialer := config.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}

	switch location.Scheme {
	case "ws":
		port := location.Port()
		if port == "" {
			port = "80"
		}
		return dialer.DialContext(ctx, "tcp", net.JoinHostPort(location.Host, port))
	case "wss":
		port := location.Port()
		if port == "" {
			port = "443"
		}
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    config.TlsConfig,
		}
		return tlsDialer.DialContext(ctx, "tcp", net.JoinHostPort(location.Host, port))
	default:
		return nil, websocket.ErrBadScheme
	}
}
