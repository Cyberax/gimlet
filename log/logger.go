package log

import (
	"fmt"
	"github.com/Cyberax/gimlet/mgsproto"
	"log"
	"unicode"
)

type GimletLogger struct {
	LogRecvPacket func(message *mgsproto.ClientMessage)
	LogSentPacket func(message *mgsproto.ClientMessage, attempt uint32)
	LogError      func(string)
	LogFlag       func(string)
	LogInfo       func(...any)
}

func NoLogging() *GimletLogger {
	return &GimletLogger{
		LogRecvPacket: func(message *mgsproto.ClientMessage) {},
		LogSentPacket: func(message *mgsproto.ClientMessage, attempt uint32) {},
		LogError:      func(string) {},
		LogFlag:       func(string) {},
		LogInfo:       func(...any) {},
	}
}

func EnableAllLogs(logData bool) *GimletLogger {
	res := &GimletLogger{
		LogRecvPacket: func(message *mgsproto.ClientMessage) {
			logPacket("IN", logData, message)
		},
		LogSentPacket: func(message *mgsproto.ClientMessage, attempt uint32) {
			logPacket(fmt.Sprintf("OUT[%d]", attempt), logData, message)
		},
		LogError: func(s string) {
			log.Default().Println("Error: ", s)
		},
		LogFlag: func(s string) {
			log.Default().Println("Flag received: ", s)
		},
		LogInfo: func(s ...any) {
			log.Default().Println(s...)
		},
	}
	return res
}

func logPacket(dir string, logData bool, msg *mgsproto.ClientMessage) {

	fields := []any{
		dir,
		"MsgType:", msg.MessageType,
		"SeqNum:", msg.SequenceNumber,
		"Flags:", msg.Flags,
		"Date:", msg.CreatedDate,
		"MsgId:", msg.MessageId.String(),
		"PayloadType:", msg.PayloadType.String(),
		"PayloadLength:", msg.PayloadLength,
	}

	if msg.PayloadType == mgsproto.Output {
		if logData || len(msg.Payload) < 16 {
			fields = append(fields, "Payload:", format(msg.Payload))
		}
	} else {
		fields = append(fields, "Payload:", format(msg.Payload))
	}

	log.Default().Println(fields...)
}

func format(payload []byte) string {
	for _, i := range payload {
		if i > unicode.MaxASCII || i < 32 {
			return fmt.Sprintf("%v", payload)
		}
	}
	return string(payload)
}
