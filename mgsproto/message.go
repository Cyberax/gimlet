// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// Package mgsproto contains cleaned up protocol specification based on the message.go file in the
// https://github.com/aws/session-manager-plugin project. However, it does not use any of the functionality
// from that package.
package mgsproto

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
)

const (
	// InputStreamMessage represents message type for input data
	InputStreamMessage = "input_stream_data"

	// OutputStreamMessage represents message type for output data
	OutputStreamMessage = "output_stream_data"

	// AcknowledgeMessage represents message type for acknowledge
	AcknowledgeMessage = "acknowledge"

	// ChannelClosedMessage represents message type for ChannelClosed
	ChannelClosedMessage = "channel_closed"

	// StartPublicationMessage represents the message type that notifies the CLI to start sending stream messages
	// AB: this appears to be almost entirely unused. The official plugin simply ignores these messages.
	StartPublicationMessage = "start_publication"

	// PausePublicationMessage represents the message type that notifies the CLI to pause sending stream messages
	// as the remote data channel is inactive
	// AB: this appears to be almost entirely unused. The official plugin simply ignores these messages.
	PausePublicationMessage = "pause_publication"
)

// AcknowledgeContent is used to inform the sender of an acknowledgment message that the message has been received.
// * MessageType is a 32 byte UTF-8 string containing the message type.
// * MessageId is a 40 byte UTF-8 string containing the UUID identifying this message being acknowledged.
// * SequenceNumber is an 8 byte integer containing the message sequence number for serialized message.
// * IsSequentialMessage is a boolean field representing whether the acknowledged message is part of a sequence,
// * it is always true for now.
type AcknowledgeContent struct {
	MessageType         string `json:"AcknowledgedMessageType"`
	MessageId           string `json:"AcknowledgedMessageId"`
	SequenceNumber      int64  `json:"AcknowledgedMessageSequenceNumber"`
	IsSequentialMessage bool   `json:"IsSequentialMessage"`
}

// ChannelClosed is used to inform the client to close the channel
// * MessageId is a 40 byte UTF-8 string containing the UUID identifying this message.
// * CreatedDate is a string field containing the message create epoch millis in UTC.
// * DestinationId is a string field containing the session target.
// * SessionId is a string field representing which session to close.
// * MessageType is a 32 byte UTF-8 string containing the message type.
// * SchemaVersion is a 4 byte integer containing the message schema version number.
// * Output is a string field containing the error message for channel close.
type ChannelClosed struct {
	MessageId     string `json:"MessageId"`
	CreatedDate   string `json:"CreatedDate"`
	DestinationId string `json:"DestinationId"`
	SessionId     string `json:"SessionId"`
	MessageType   string `json:"MessageType"`
	SchemaVersion int    `json:"SchemaVersion"`
	Output        string `json:"Output"`
}

type PayloadType uint32

const (
	Output                       PayloadType = 1
	Error                        PayloadType = 2
	Size                         PayloadType = 3
	Parameter                    PayloadType = 4
	HandshakeRequestPayloadType  PayloadType = 5
	HandshakeResponsePayloadType PayloadType = 6
	HandshakeCompletePayloadType PayloadType = 7
	EncChallengeRequest          PayloadType = 8
	EncChallengeResponse         PayloadType = 9
	Flag                         PayloadType = 10
	StdErr                       PayloadType = 11
	ExitCode                     PayloadType = 12
)

func (t PayloadType) String() string {
	switch t {
	case Output:
		return "Output"
	case Error:
		return "Error"
	case Size:
		return "Size"
	case Parameter:
		return "Parameter"
	case HandshakeRequestPayloadType:
		return "HandshakeRequestPayloadType"
	case HandshakeResponsePayloadType:
		return "HandshakeResponsePayloadType"
	case HandshakeCompletePayloadType:
		return "HandshakeCompletePayloadType"
	case EncChallengeRequest:
		return "EncChallengeRequest"
	case EncChallengeResponse:
		return "EncChallengeResponse"
	case Flag:
		return "Flag"
	case StdErr:
		return "StdErr"
	case ExitCode:
		return "ExitCode"
	default:
		return fmt.Sprintf("%d", t)
	}
}

type FlagMessage uint32

const (
	DisconnectToPort   FlagMessage = 1
	TerminateSession   FlagMessage = 2
	ConnectToPortError FlagMessage = 3
)

// ClientMessage represents a message for client to send/receive.
// ClientMessage Message in MGS is equivalent to MDS' InstanceMessage.
// All client messages are sent in this form to the MGS service.
// * HL - HeaderLength is a 4 byte integer that represents the header length (not including the HL field itself)
// * MessageType is a 32 byte UTF-8 string containing the message type.
// * SchemaVersion is a 4 byte integer containing the message schema version number.
// * CreatedDate is an 8 byte integer containing the message create epoch millis in UTC.
// * SequenceNumber is an 8 byte integer containing the message sequence number for serialized message streams.
// * Flags is an 8 byte unsigned integer containing a packed array of control flags:
// *   Bit 0 is SYN - SYN is set (1) when the recipient should consider Seq to be the first message number in the stream
// *   Bit 1 is FIN - FIN is set (1) when this message is the final message in the sequence.
// *   In practice, we only set Flags to SYN|FIN (3) for the `Acknowledge` and `ChannelClosed` messages.
// *   Everywhere else it's 0.
// * MessageId is a 16 byte random UUID identifying this message. (AB: I fixed the incorrect definition)
// * Payload digest is a 32 byte containing the SHA-256 hash of the payload.
// * PayloadLength is a 4 byte unsigned integer containing the byte length of data in the Payload field.
//   It's not a reliable field and appears to contain garbage for some message types.
// * Payload is a variable length byte data.
//
// The maximum payload length for streaming data appears to be 1024 bytes
//
// All numbers are sent as big-endian down the wire.
type ClientMessage struct {
	HeaderLength   uint32
	MessageType    string // 32 bytes
	SchemaVersion  uint32
	CreatedDate    uint64 // Unix time in milliseconds
	SequenceNumber int64
	Flags          ClientMessageFlag // uint64
	MessageId      uuid.UUID         // 16 bytes
	PayloadDigest  []byte            // 32 bytes (sha256)
	PayloadType    PayloadType       // uint32
	PayloadLength  uint32
	Payload        []byte // Variable length
}

const MaxStreamingPayloadLength = 1024

type ClientMessageFlag uint64

const ClientMessageFlagSYN = 1
const ClientMessageFlagFIN = 2

func (c ClientMessageFlag) String() string {
	res := ""
	if c&ClientMessageFlagSYN != 0 {
		res += "SYN"
	}
	if c&ClientMessageFlagFIN != 0 {
		if res != "" {
			res += "|"
		}
		res += "FIN"
	}
	if res == "" {
		res = "0"
	}
	return res
}

// OpenDataChannelInput is sent as a WebSocket text message to initiate the SSM session. It must be
// the very first message sent over a newly opened WebSocket.
type OpenDataChannelInput struct {
	_ struct{} `type:"structure"`

	MessageSchemaVersion string `json:"MessageSchemaVersion" min:"1" type:"string" required:"true"`
	RequestId            string `json:"RequestId" min:"16" type:"string" required:"true"`
	TokenValue           string `json:"TokenValue" min:"1" type:"string" required:"true"`
	ClientId             string `json:"ClientId" min:"1" type:"string" required:"true"`
}

const SchemaVersion = "1.0"
const SchemaVersionNum = 1

////////////////////////////////////////////////////////////////////////////////////////////////
/////////////////////////////// HANDSHAKE-RELATED  /////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////

type ActionType string

const (
	KMSEncryption ActionType = "KMSEncryption"
	SessionType   ActionType = "SessionType"
)

type ActionStatus int

const (
	Success     ActionStatus = 1
	Failed      ActionStatus = 2
	Unsupported ActionStatus = 3
)

// SessionTypeRequest request contains type of the session that needs to be launched and properties for plugin
type SessionTypeRequest struct {
	SessionType string      `json:"SessionType"`
	Properties  interface{} `json:"Properties"`
}

// HandshakeRequestPayload payload sent by the agent to the session manager plugin
type HandshakeRequestPayload struct {
	AgentVersion           string                  `json:"AgentVersion"`
	RequestedClientActions []RequestedClientAction `json:"RequestedClientActions"`
}

// RequestedClientAction an action requested by the agent to the plugin
type RequestedClientAction struct {
	ActionType       ActionType      `json:"ActionType"`
	ActionParameters json.RawMessage `json:"ActionParameters"`
}

// ProcessedClientAction The result of processing the action by the plugin
type ProcessedClientAction struct {
	ActionType   ActionType   `json:"ActionType"`
	ActionStatus ActionStatus `json:"ActionStatus"`
	ActionResult interface{}  `json:"ActionResult"`
	Error        string       `json:"Error"`
}

// HandshakeResponsePayload is sent by the plugin in response to the handshake request
type HandshakeResponsePayload struct {
	ClientVersion          string                  `json:"ClientVersion"`
	ProcessedClientActions []ProcessedClientAction `json:"ProcessedClientActions"`
	Errors                 []string                `json:"Errors"`
}

// GimletVersion the version of our "plugin", sent during the handshake
const GimletVersion = "1.2.0.0-gimlet"
