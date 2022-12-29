package internal

import (
	"github.com/google/uuid"
	"time"
)

type WsConn interface {
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	ReadMessage() ([]byte, error)
	WriteMessage(data []byte) error
	WriteText(data string) error
	SendPing() error
}

type ErrorCallbackFunc func(err error)
type AckReceiver func(messageType string, ackedMsgId uuid.UUID, ackedSequenceNumber int64)
