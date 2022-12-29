package gimlet

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/Cyberax/gimlet/internal"
	"github.com/Cyberax/gimlet/internal/utils"
	"github.com/Cyberax/gimlet/log"
	"github.com/Cyberax/gimlet/mgsproto"
	"github.com/xtaci/smux"
	"golang.org/x/net/websocket"
	"io"
	"sync"
)

type Channel struct {
	options ChannelOptions

	mtx  sync.Mutex
	conn *websocket.Conn
	done chan bool

	// This channel is signalled when we either get the muxer running
	// or if we end up with an error set.
	handshakeComplete chan bool
	err               error

	sender   *internal.Sender
	receiver *internal.Receiver
	muxer    *smux.Session

	stats log.Stats
}

// NewChannel creates a new Gimlet connection to the specified endpoint, the websocket connection
// uses the standard defaults. `ctx` parameter can be used for cancellation or deadline support.
func NewChannel(ctx context.Context, endpoint string, token string, options ChannelOptions) (*Channel, error) {
	config, err := websocket.NewConfig(endpoint, endpoint)
	if err != nil {
		return nil, err
	}

	conn, err := internal.DialWebSocket(ctx, config)
	if err != nil {
		return nil, err
	}

	channel := NewChannelWithConnection(conn, token, options)
	return channel, nil
}

// NewChannelWithConnection creates a new Gimlet channel over a caller-provided Websocket connection. The function
// still does the full MGS handshake.
func NewChannelWithConnection(conn *websocket.Conn, token string, options ChannelOptions) *Channel {
	wc := &Channel{
		options:           options,
		done:              make(chan bool),
		handshakeComplete: make(chan bool),
		conn:              conn,
	}

	wsAdapter := internal.NewWSAdapter(conn)

	wc.sender = internal.NewSender(wsAdapter, wc.close,
		options.MaxPacketsPerSecond, options.ResendTimeout, options.Log, &wc.stats)
	wc.receiver = internal.NewReceiver(wsAdapter, wc.close, wc.sender.EnqueueAck, options.Log, &wc.stats)

	wc.options.Log.LogInfo("Starting connection to token: " + token)
	wc.sender.EnqueueChannelOpen(token)

	go wc.runControlQueueReader()

	return wc
}

// GetStats returns the object that tracks live channel statistics
func (c *Channel) GetStats() *log.Stats {
	return &c.stats
}

// WaitUntilReady waits for the MGS handshake to complete
func (c *Channel) WaitUntilReady() error {
	<-c.handshakeComplete
	return c.GetError()
}

// GetHandshakeChannel returns the channel that becomes readable when the MGS handshake is complete.
// You can use it to monitor for the completion in a `select` clause. Check the `GetError` method to
// get the result of the handshake.
func (c *Channel) GetHandshakeChannel() <-chan bool {
	return c.handshakeComplete
}

// GetError returns the error that has caused the connection to be closed. Normal closure (via `Shutdown` method)
// is signalled by the `io.EOF` error.
// It returns `nil` while the channel is open.
func (c *Channel) GetError() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	return c.err
}

// GetDieChan returns the channel that becomes readable once the channel is completely terminated
func (c *Channel) GetDieChan() <-chan bool {
	return c.done
}

// Terminate closes the connection and frees all resources taken by it. It does not block waiting
// for an orderly shutdown.
func (c *Channel) Terminate() {
	c.close(io.ErrClosedPipe)
}

// OpenStream creates a new channel to the target port. If the connection is not yet hands
//
// The returned `smux.Stream` implements the `net.Conn` interface and can be used in any methods that accept it.
// If the SSM agent can't establish the connection, the returned `smux.Stream` will immediately fail with an
// error on the first `Read` operation.
func (c *Channel) OpenStream() (*smux.Stream, error) {
	err := c.WaitUntilReady() // Wait for the muxer to be set
	if err != nil {
		return nil, err
	}

	// If there's no error then the muxer is ready
	utils.PanicIf(c.muxer == nil, "muxer is not set")
	return c.muxer.OpenStream()
}

func (c *Channel) close(err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if !utils.IsChannelOpen(c.done) {
		return
	}

	if err == io.EOF {
		c.options.Log.LogInfo("channel was closed gracefully")
	} else {
		c.options.Log.LogError("unexpected channel closure: " + err.Error())
	}

	c.err = err

	if c.muxer != nil {
		_ = c.muxer.Close()
	}

	c.sender.Shutdown()
	c.receiver.Shutdown()
	_ = c.conn.Close()
	if utils.IsChannelOpen(c.handshakeComplete) {
		close(c.handshakeComplete)
	}
	close(c.done)
}

// Shutdown attempts to close the connection gracefully. It sends a `TerminateSession` message and
// then waits for the connection to be closed. `ctx context.Context` can be used for deadline/cancellation.
// If the deadline specified in the context expires before the connection is closed, the connection is forcibly
// terminated. All resources are freed by the time this method returns.
func (c *Channel) Shutdown(ctx context.Context) {
	_ = c.sender.EnqueueControlFlag(mgsproto.TerminateSession)

	select {
	case <-c.done:
	case <-ctx.Done():
	}

	c.Terminate()
}

func (c *Channel) runControlQueueReader() {
loop:
	for {
		ch := c.receiver.ControlReaderQueue().ReadChan()
		select {
		case msg := <-ch:
			if msg != nil {
				c.dispatch(msg)
			}
		case <-c.done:
			break loop
		}
	}
}

func (c *Channel) dispatch(msg *mgsproto.ClientMessage) {
	if msg.MessageType == mgsproto.PausePublicationMessage {
		c.options.Log.LogInfo("treating pause_publication as channel_closed")
		c.close(io.EOF)
		return
	}

	if msg.MessageType == mgsproto.StartPublicationMessage {
		// Nothing to do here
		return
	}

	if msg.MessageType == mgsproto.AcknowledgeMessage {
		c.handleAck(msg.Payload)
		return
	}

	if msg.MessageType == mgsproto.ChannelClosedMessage {
		c.options.Log.LogInfo("channel_closed received from the server")
		c.close(io.EOF)
		return
	}

	if msg.MessageType == mgsproto.OutputStreamMessage {
		switch msg.PayloadType {
		case mgsproto.HandshakeRequestPayloadType:
			c.handleHandshake(msg)
		case mgsproto.HandshakeCompletePayloadType:
			c.runMuxer()
		case mgsproto.Flag:
			c.handleFlag(msg)
		default:
			c.stats.NumUnknownPackets.Add(1)
			c.options.Log.LogInfo("Unknown packet type ignored", msg.PayloadType.String())
		}
	}

}

func (c *Channel) handleAck(payload []byte) {
	ack := mgsproto.AcknowledgeContent{}
	err := json.Unmarshal(payload, &ack)
	if err != nil {
		err = fmt.Errorf("failed to parse an ack: %w", err)
		c.close(err)
		return
	}
	c.sender.AckReceived(ack.SequenceNumber)
}

func (c *Channel) handleFlag(msg *mgsproto.ClientMessage) {
	if len(msg.Payload) != 4 {
		c.stats.NumUnknownPackets.Add(1)
		c.options.Log.LogInfo("Malformed flag ignored")
		return
	}

	c.stats.NumFlags.Add(1)

	flag := mgsproto.FlagMessage(binary.BigEndian.Uint32(msg.Payload))
	if flag == mgsproto.TerminateSession {
		c.options.Log.LogInfo("Terminate session received")
	}
	if flag == mgsproto.ConnectToPortError {
		c.options.Log.LogInfo("Port error")
		c.options.OnConnectionFailedFlagReceived()
	}
}

func (c *Channel) runMuxer() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// `runMuxer` can be called more than once if the server retries the message
	if c.muxer != nil || c.err != nil {
		return
	}

	rw := internal.NewReadWriter(c.receiver.DataReaderQueue(), c.sender.EnqueueData)

	var err error
	c.muxer, err = smux.Client(rw, c.options.SmuxConfig)
	utils.PanicIfErr(err) // Error can only happen if `cfg` is malformed

	close(c.handshakeComplete)
}

func (c *Channel) handleHandshake(msg *mgsproto.ClientMessage) {
	hrp := mgsproto.HandshakeRequestPayload{}

	err := json.Unmarshal(msg.Payload, &hrp)
	if err != nil {
		c.close(fmt.Errorf("failed to unmarshal the handshake request: %w", err))
		return
	}
	if len(hrp.RequestedClientActions) != 1 {
		c.close(fmt.Errorf("handshake request has more than 1 action"))
		return
	}
	if hrp.RequestedClientActions[0].ActionType != mgsproto.SessionType {
		c.close(fmt.Errorf("unknown action specified"))
		return
	}

	var handshakeResponse mgsproto.HandshakeResponsePayload
	handshakeResponse.ClientVersion = mgsproto.GimletVersion
	handshakeResponse.ProcessedClientActions = []mgsproto.ProcessedClientAction{}

	processedAction1 := mgsproto.ProcessedClientAction{}
	processedAction1.ActionType = mgsproto.SessionType
	processedAction1.ActionStatus = mgsproto.Success
	handshakeResponse.ProcessedClientActions = append(handshakeResponse.ProcessedClientActions, processedAction1)

	err = c.sender.SerializeAndEnqueueControl(mgsproto.HandshakeResponsePayloadType, &handshakeResponse)
	if err != nil {
		c.close(err)
		return
	}
}
