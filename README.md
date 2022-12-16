# Gimlet - poke holes through the VPC veil in AWS

This library can be used to poke (small) holes in AWS VPC, primarily for various low-bandwidth control-plane
functionality. For example, it can be used to tunnel the SSH or HTTP traffic from an AWS instance to your local
host. It uses something that is called "MGS protocol" internally in the SSM codebase.

The official AWS CLI provides similar functionality as a part of the `session-manager-plugin` 
tool: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html
However, AWS doesn't package this functionality in a reusable library.

Gimlet is intended to be primarily used as a Go library, and it's designed to have minimal dependencies.
The `gimlet-proxy` tool is provided as an example of the library use and can also be used as a small stand-alone 
tool to do port forwarding.

The library is named after a tool called "gimlet", not after an alcoholic cocktail:

![Gimlet](docs/gimlet.jpg)

Although it feels like quite a bit of alcohol was involved in designing the MGS protocol, as you can see from its 
description below. 

# Running `gimlet-proxy`

`gimlet-proxy` provides an easy way to test the functionality of the Gimlet library. It also serves as a sample of the
Gimlet library use.

You can run `gimlet-proxy` by using `go run` like this:

```
cd gimlet-proxy
go run gimlet-proxy.go
```

There are following command-line options:

| Option       | Description                                                                    |
|--------------|--------------------------------------------------------------------------------|
| -debug       | Print the MGS protocol messages                                                |
| -profile     | The AWS profile to use, if empty then AWS_* environment variables will be used |
| -listen-port | The port to listen on your computer                                            |
| -instance-id | The EC2 instance to connect to                                                 |
| -target-host | The host to connect to, empty for the EC2 instance itself                      |
| -target-port | The target port                                                                |

The `-instance-id` is the EC2 instance that you want to use for the connection. You can connect to a port on this
EC2 instance itself by leaving the `-target-host` empty, or you can use this EC2 to connect to another instance 
within the same VPC by specifying a non-empty `-target-host`. Once started, `gimlet-proxy` prints out its statistics 
every 10 seconds.

Below are some examples of `gimlet-proxy` use.

### Connecting to SSH on the target instance

Run the gimlet proxy:

```
$ cd gimlet-proxy
$ go run gimlet-proxy.go --profile mine --instance-id i-031b47513614f3e63 -target-port 22 -listen-port 2222
```

Then use SSH to connect to the instance (via localhost):

```
cyberax@MyHost:~$ ssh -p 2222 ubuntu@localhost
Warning: Permanently added '[localhost]:2222' (ED25519) to the list of known hosts.
Welcome to Ubuntu 22.04 LTS (GNU/Linux 5.15.0-1026-aws x86_64)

* Documentation:  https://help.ubuntu.com
...
```

You can also use SCP to copy data to and from the instance:

```
cyberax@MyHost:~$ scp -P 2222 ubuntu@localhost:/tmp/testfile .
...
```

This is equivalent to AWS CLI command:

```
aws --profile mine ssm start-session \ 
  --target i-031b47513614f3e63 --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["22"], "localPortNumber":["2222"]}'
```

### Forwarding a remote HTTP server via an EC2 instance

Run the `gimlet-proxy` tool:

```
$ cd gimlet-proxy
$ go run gimlet-proxy.go --profile mine --instance-id i-031b47513614f3e63 \ 
    -target-host 172.51.65.12 -target-port 80 -listen-port 8080
```

And then use a browser to access it via `http://localhost:8080`.

### Connecting to an RDS instance that doesn't have a public endpoint

Run the `gimlet-proxy` tool:

```
$ cd gimlet-proxy
$ go run gimlet-proxy.go --profile mine --instance-id i-031b47513614f3e63 \ 
    -target-host demo-db.ci12341234bp.us-east-2.rds.amazonaws.com -target-port 5432 -listen-port 5432
```

And then use `psql` tool to connect to it: `psql host=localhost`

# Using the library

Gimlet was meant to be used as a library in your project, rather than a stand-alone tool. It was quite carefully 
designed to have as few external dependencies as possible. As such, it's split into two modules `pierce` and `gimlet`.

## Pierce 

`github.com/Cyberax/gimlet/pierce` module is used to prepare the connection by establishing the SSM session.  

```go
package main

import (
	"context"
	"github.com/Cyberax/gimlet/pierce"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func prepare(config aws.Config) (*pierce.ConnInfo, error) {
	// Prepare the connection to 172.16.22.44:22 via EC2 instance i-123123123213
	connInfo, err := pierce.PierceVpcVeil(context.Background(), config, "i-123123123213", "172.16.22.44", 22)
	if err != nil {
		return nil, err
    }
    return connInfo, nil
}
```

The `PierceVpcVeil` method returns a `ConnInfo` object that contains information about the session being established.
This is a very simple data object:

```go
type ConnInfo struct {
	InstanceId string
	Region     string
	Endpoint   string

	SessionId string
	Token     string
}
```

It can be serialized (via JSON or any other means) and communicated to a remote host. The `ConnInfo` object can only
be used only once to establish a Gimlet connection, and it doesn't grant any additional AWS privileges. Here's an
example of a token:

```
InstanceId = "i-123123123213"
Region = "us-west-2"
Endpoint = "wss://ssmmessages.us-west-2.amazonaws.com/v1/data-channel/cyberax-asdfas-03412341221231?role"
SessionId = "cyberax-asdfas-03412341221231"
Token = "AAEAAfUIg5GFQ.....kR"
```

## Open Channel

Once you have the token, you can use it to open a communication channel with the target. This is done with the help
of the `github.com/Cyberax/gimlet` module. It's a separate module from `pierce` to avoid pulling in the AWS SDK for
the users of the `gimlet` module. 

The entry point is `gimlet.NewChannel`. This function initiates the WebSocket connection to the MGS endpoint and 
initializes the connection:

```go
import "github.com/Cyberax/gimlet"

var ci *gimlet.ConnInfo
ci = deserializeConnInfo() // Obtain the connection info

options := gimlet.DefaultChannelOptions()
channel, err := gimlet.NewChannel(context.Background(), ci.Endpoint, ci.Token, options)
if err != nil {
	return nil, err
}
defer channel.Shutdown(context.Background()) // Make sure resources are freed
```

This `NewChannel` accept a `context.Context` that can be used to cancel the pending WebSocket connection. 

It also accepts a `gimlet.ChannelOptions` that specifies various channel settings. You can use this object to modify 
the `Dialer` interface used by the WebSocket library to establish the connection, which might be useful in tests.

The `NewChannel` function starts background goroutines, so you need to make sure that you clean up the resources by
calling `Terminate` or `Shutdown` methods of `gimlet.Channel` when you're finished with it.

These background goroutines perform the full MGS negotiation, which can take some time. You can use `WaitUntilReady`
method to wait for the handshake to complete, or use `GetHandshakeChannel` to get a channel that becomes readable
on the handshake completion (successful or erroneous).

```go
// Option 1
err = channel.WaitUntilReady()
if err != nil {
	return err
}
// Option 2
<- channel.GetHandshakeChannel()
err = channel.GetError()
if err != nil {
	return err
}
```

After that, you're ready to start using the channel by opening connections using the `OpenStream` method:

```go
stream1, err := channel.OpenStream()
if err != nil {
	return err
}
stream1.Read(...)
stream1.Write(...)
defer stream1.Close()

stream2, err := channel.OpenStream()
....
```

`channel.OpenStream` method returns objects of type `*smux.Stream` that fully implement the `net.Conn` interface,
including support for the read/write deadlines. You can also use one channel to open multiple concurrent connections. 
Under the hood, MGS uses a wonderfully simple TCP multiplexer: https://github.com/xtaci/smux/

And that's it! Recapping, the full SSM connection establishment can be done in just a few lines of code:

```go
channel, err := gimlet.NewChannel(context.Background(), ci.Endpoint, ci.Token, gimlet.DefaultChannelOptions())
if err != nil {
	return nil, err
}
defer channel.Shutdown(context.Background())
stream, err := channel.OpenStream()
if err != nil {
	return nil, err
}
```

# Debugging, stats and rate-limiting

You can enable debugging by using `gimlet.DebugChannelOptions()` instead `gimlet.DefaultChannelOptions()`. This will
enable debug logging for the MGS protocol messages. The default debug logger uses the standard `log` Go package,
you can plug in your own logger by customizing the `GimletLogger` structure.

Gimlet also provides a way to get statistics, via the `GetStats` method. It returns an object that contains atomic 
counters that are live-updated as long as the channel stays open.

MGS protocol appears to be hard rate-limited to 1000 data packets per second. So Gimlet uses 900 packets per second
to have a bit of a safety margin. This is configurable via `MaxPacketsPerSecond` in `ChannelOptions`, if AWS ever
changes this limit.

# FAQ and TODOs

_Q_: Is it unsafe?!? Are you telling me that anyone with access to AWS credentials can connect to any of 
my precious RDS databases?

_A_: Gimlet does not expose anything that can't be accessed right now via AWS CLI and `session-manager-plugin`. So yes,
if you're solely dependent on VPC to isolate the databases from attackers, you need to make sure that SSM is disabled
for your AWS accounts.

_Q_: What about security groups and network ACLs?

_A_: If you're connecting to a service that runs on the target EC2 instance itself, then security groups or ACLs 
do not apply. In fact, you can connect to a service that is bound to a loopback interface. If you're using the
EC2 instance to connect to a different host, then all the intra-VPC security groups and ACLs apply.

_Q_: What are the limits? What is the bandwidth?

_A_: It appears that MGS protocol is limited to around 1 megabyte per second for one session. Though it's possible 
to open multiple concurrent sessions to one host for up to about 10 megabytes per second.

_Q_: How reliable is it?

_A_: Gimlet is pretty stable for control-plane like functionality (e.g. SSH for debugging). You probably shouldn't
use it for high-bandwidth applications, though.

_Q_: What are the session duration limits?

_A_: I've tested it with 24-hour long sessions. It doesn't appear that the sessions themselves have a limit, unless
it's explicitly configured via the SSM console. 

_Q_: What's currently missing? What are the improvement areas?

_A_: Currently there is no flow control, so the library suffers from extreme bufferbloat (see:
https://en.wikipedia.org/wiki/Bufferbloat ) in case you want to run a high-bandwidth stream (e.g. an `scp` transfer)
in parallel with an interactive session. Fixing this will require adopting some queue size control method,
possibly a variation of CoDel.

_Q_: Tests?

_A_: 😭Patches welcome.

# MGS protocol description

The MGS protocol is, to say the least, peculiar. It appears to be designed by a committee and then implemented by
teams that don't speak with each other.

The protocol can be reverse-engineered by reading the publicly released `session-manager-plugin` source code. In
particular, message definitions are here: https://github.com/aws/session-manager-plugin/tree/mainline/src/message

## Handshake

The client starts by establishing a WebSocket connection to the endpoint returned from the `ssm.StartSession` API call.
The then initiates the protocol handshake by sending a text WebSocket message that contains the JSON for the
`OpenDataChannelInput` structure:

```json
{
  "MessageSchemaVersion": "1.0",
  "RequestId": "d153ec1b-09ca-46f4-a5ff-93e1ecf2b1c2",
  "TokenValue": "AAEAA.....",
  "ClientId":"ca23bc98-d34f-4815-9e4c-f33953868562"
}
```

The `RequestId` and `ClientId` should be set to randomly generated UUIDs, while `TokenValue` is set to the value 
returned from the `ssm.StartSession` call.

From that point on, the client must use `ClientMessage` structures to send and receive information. These structures
are binary-serialized, see the notes below.

The handshake then continues, and the client needs to read the `ClientMessage` with the following parameters:
```
ClientMessage::MessageType = "output_stream_data"
ClientMessage::PayloadType = HandshakeRequestPayloadType (=5)
```

The payload of the message contains JSON-serialized `mgsproto.HandshakeRequestPayload` structure:
```json
{
  "AgentVersion":"3.1.1732.0",
  "RequestedClientActions":
  [
    {"ActionType":"SessionType","ActionParameters": {"SessionType":"Port","Properties": 
    {"host":"172.31.25.54","localPortNumber":"7406","portNumber":"3000","type":"LocalPortForwarding"}}}
  ]
}
```

There can theoretically be multiple actions, but we support only the lone `SessionType=Port` sessions.

The client replies with its `ClientMessage`:

```
ClientMessage::MessageType = "input_stream_data"
ClientMessage::PayloadType = HandshakeResponsePayloadType (=6)
```

The payload of this message contains JSON-serialized `mgsproto.HandshakeResponsePayload`:
```json
{
  "ClientVersion":"1.2.0.0",
  "ProcessedClientActions":[{"ActionType":"SessionType","ActionStatus":1,"ActionResult":null,"Error":""}],
  "Errors":null
}
```

The server then replies with the final: 

```
ClientMessage::MessageType = "input_stream_data"
ClientMessage::PayloadType = HandshakeCompletePayloadType (=7)
```

This signifies the completion of the handshake, and after that the connection traffic can start to flow. 

## Payload

The payload traffic is transmitted from the server to client in: 
```
ClientMessage::MessageType = "output_stream_data"
ClientMessage::PayloadType = Output (=1)
```

The payload traffic from the client to server is transmitted in:
```
ClientMessage::MessageType = "input_stream_data"
ClientMessage::PayloadType = Output (=1)
```

## Flow control

Client messages of type `input_stream_data` and `output_stream_data` use sequence-ID based flow control. Each 
`ClientMessage` needs to have correct `SequenceId` field. The sequence IDs start with 1, and are incremented for
each outgoing message.

When a client receives an `output_stream_data` message it needs to send an acknowledgment message (ack) to the server.
The ack message contains the `SequenceId` and the `MessageId` of the received message.

This ack message has the following structure:

```
ClientMessage::MessageType = "acknowledge"
ClientMessage::PayloadType = 0
```

The payload for this message is a JSON-serialized `AcknowledgeContent` structure:

```json
{
  "AcknowledgedMessageType":"input_stream_data",
  "AcknowledgedMessageId":"229b0a43-0e89-4078-b0f1-8b5431feee93",
  "AcknowledgedMessageSequenceNumber":1697,
  "IsSequentialMessage":true
}
```

Yes, acks are transmitted as huge JSON structures that even contain the entirely useless `"IsSequentialMessage":true`
field whose value is hard-coded.

In turn, the client needs to await acknowledgement messages from the server for each of its `input_stream_data` 
`ClientMessages`. If no ack is received within the specified timeout, the client must re-transmit the client message.

The timeout is statically configured at 1.5 seconds right now. Packet loss almost never happens, so doing anything
much more complicated is not necessary.

*NOTE:* _ONLY_ messages type `input_stream_data` and `output_stream_data` use flow control. All other message types
do NOT use it, even if they have `ClientMessage::SequenceId` field set. So these messages must NOT have their 
SeqId field be used for flow control: `acknowledge`, `channel_closed`, `start_publication`, `pause_publication`.

## Channel teardown

The `channel_closed` message is sent by the server when the channel has been closed by either side:

```
ClientMessage::MessageType = "channel_closed"
ClientMessage::PayloadType = 261
```

It's safe to immediately tear down the connection as a result.

## Flags

There is one outgoing flag message that Gimlet can send to initiate the tear-down of the connection:

```
ClientMessage::MessageType = "input_stream_data"
ClientMessage::PayloadType = Flag (=10)
```

The payload of the message is a big-endian 4-byte value of 2 ([0 0 0 2]).

There is one incoming flag message, the `ConnectToPortError` flag. It's received in case the server-side fails to
connect to the target host/port combination. The flag message payload contains only 4-byte big-endian value 
of 3 ([0 0 0 3]). There is no way to correlate it with the `channel.OpenStream` call that resulted in this error.

It's thus pretty useless. Gimlet calls the `OnConnectionFailedFlagReceived` callback function specified in 
`ChannelOptions`. By default, this function does nothing.

## start_publication and pause_publication messages

The `start_publication` messages are sometimes sent as the very first messages within a session, even before the
initial handshake. It's safe to ignore them.

`pause_publication` messages are (in my tests) sent to the SSM agent when the rate of traffic becomes too high (more 
than 1000 packets per second sustained for more than 2 seconds). The SSM agent pauses outgoing traffic as a result, see: 
https://github.com/aws/amazon-ssm-agent/blob/49163b8f3bd47b3dce6e8f97e04d58a7874bcc6e/agent/session/datachannel/datachannel.go#L769

Judging from the name, `start_publication` is supposed to resume the flow, but in my experiments it is never received
before the ping timeouts kill the connection.

Another wrinkle is that `pause_publication` is sent to the client (Gimlet) only when the server-side connection 
is closed. So it's pretty safe to treat `pause_publication` as the `channel_closed` message. 

## Rate limiting

I've spent quite a bit of time poring over the source code of the SSM agent and the `session-manager-plugin` in search
of the rate limiting method that they use.

Is it CoDel? Some variant of Cubic? Something else? The answer turned out to be this:

```go
time.Sleep(time.Millisecond)
```

Both in the SSM agent and the session manager plugin:
https://github.com/aws/session-manager-plugin/blob/c523002ee02c8b68983ad05042ed52c44d867952/src/sessionmanagerplugin/session/portsession/muxportforwarding.go#L223
https://github.com/aws/amazon-ssm-agent/blob/c4414a04a161ed90e141050fb1a8cc7f43835e70/agent/session/plugins/port/port_mux.go#L247

Basically, there is no explicit flow control. The 1-millisecond delay between reading packets results in an 
effective maximum packet rate of 1000 per second.

My conscience simply doesn't allow me to use this kind of dirty hacks. So Gimlet implements a simple token-bucket based
flow pacer. It's limited to 900 packets per second, but it actually achieves slightly higher flow rate than the 
regular SSM session plugin (around 900 kbps sustained versus ~680 kbps sustained). There's nothing to be done about
the SSM agent side, though.

Exceeding the 1000 packets-per-second limit results in swift WebSocket connection termination by the AWS. 

## ClientMessage serialization

ClientMessage is described in the `mgsproto/message.go` file. Please consult it for possible values of `PayloadType`.
The messages are variable-sized structures with payload that follows the header. 

Here's the header format:

```go
type ClientMessage struct {
	HeaderLength   uint32
	MessageType    string // 32 bytes
	SchemaVersion  uint32
	CreatedDate    uint64 // Unix time in milliseconds
	SequenceNumber int64 // SequenceId
	Flags          ClientMessageFlag // uint64
	MessageId      uuid.UUID         // 16 bytes
	PayloadDigest  []byte            // 32 bytes (sha256)
	PayloadType    PayloadType       // uint32
	PayloadLength  uint32
	Payload        []byte // Variable length
}
```

All integers are serialized in big-endian byte order (aka "network order").

`HeaderLength` is the header length, not counting the `HeaderLength` field itself. It's always equal to 116.

`MessageType` is the type of the message, one of: `input_stream_data`, `output_stream_data`, `acknowledge`, 
`channel_closed`, `start_publication`, `pause_publication`. The type is right-padded with spaces to 32 byte length.

`Flags` field is used to indicate the flags for the message. Can be OR-ed combination of SYN (1) or FIN (2). Currently,
this flag is set to `SYN | FIN` for messages that don't need SequenceID (i.e. anything that is not `input_stream_data`
or `output_stream_data`).

`MessageId` is the UUID with the unique message ID. The UUIDs are interpreted as 128-bit integers that are 
written in big-endian order. Here's an example:

```
Wire order:    a3de282f-478b-a6e6-812e-f34f87bd449e
Logical order: 812ef34f-87bd-449e-a3de-282f478ba6e6
```

`PayloadLength` contains the payload length. It should be equal to `len(packet) - HeaderLength - 4`. However, this
field is NOT reliable. Several message type have it set incorrectly. In particular, `start_publication` has it set 
to a little-endian serialized packet length. It's safe to ignore this field in general during the message validation.

`PayloadDigest` is the SHA-256 hash of the content. It's more reliable than `PayloadLength`, but it's still set 
incorrectly for `start_publication` and `pause_publication` messages. However, it's always correctly set for the data
packets.
