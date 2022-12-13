package gimlet

import (
	"github.com/Cyberax/gimlet/log"
	"github.com/gorilla/websocket"
	"github.com/xtaci/smux"
	"time"
)

// The safe packets-per-second amount
const safePps = 900
const resendTimeout = 1500 * time.Millisecond

type ConnInfo struct {
	InstanceId string
	Region     string
	Endpoint   string

	SessionId string
	Token     string
}

type ChannelOptions struct {
	Dialer     *websocket.Dialer
	Log        *log.GimletLogger
	SmuxConfig *smux.Config

	OnConnectionFailedFlagReceived func()

	MaxPacketsPerSecond uint64
	ResendTimeout       time.Duration
}

func DefaultChannelOptions() ChannelOptions {
	config := smux.DefaultConfig()
	return ChannelOptions{
		Dialer:     websocket.DefaultDialer,
		Log:        log.NoLogging(),
		SmuxConfig: config,

		OnConnectionFailedFlagReceived: func() {},

		MaxPacketsPerSecond: safePps,
		ResendTimeout:       resendTimeout,
	}
}

// DebugChannelOptions creates channel options with enabled MGS protocol message logging. The
// default logging implementation uses the standard `log` package. Set `logPayloadData` to
// dump the data packets' payload and not just meta-information.
func DebugChannelOptions(logPayloadData bool) ChannelOptions {
	config := smux.DefaultConfig()
	return ChannelOptions{
		Dialer:     websocket.DefaultDialer,
		Log:        log.EnableAllLogs(logPayloadData),
		SmuxConfig: config,

		OnConnectionFailedFlagReceived: func() {},

		MaxPacketsPerSecond: safePps,
		ResendTimeout:       resendTimeout,
	}
}
