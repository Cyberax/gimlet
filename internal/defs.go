package internal

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"time"
)

type WsConn interface {
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
}

var _ WsConn = &websocket.Conn{}

type ErrorCallbackFunc func(err error)
type AckReceiver func(messageType string, ackedMsgId uuid.UUID, ackedSequenceNumber int64)
