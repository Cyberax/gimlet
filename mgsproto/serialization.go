package mgsproto

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/Cyberax/gimlet/internal/utils"
	"github.com/google/uuid"
	"strings"
)

const ClientMessageHeaderLen = 116

func SerializeMGSClientMessage(msg *ClientMessage) []byte {
	payloadLength := uint32(len(msg.Payload))
	msg.PayloadLength = payloadLength

	headerLength := uint32(ClientMessageHeaderLen)

	totalMessageLength := headerLength + 4 + payloadLength
	out := bytes.NewBuffer(make([]byte, 0, totalMessageLength))

	be := binary.BigEndian
	utils.Must(binary.Write(out, be, &headerLength))

	// Pad the message type with spaces to 32 bytes
	msgType := []byte(msg.MessageType + strings.Repeat(" ", 32))[:32]
	utils.MustReturn(out.Write(msgType))

	utils.Must(binary.Write(out, be, msg.SchemaVersion))
	utils.Must(binary.Write(out, be, msg.CreatedDate))
	utils.Must(binary.Write(out, be, msg.SequenceNumber))
	utils.Must(binary.Write(out, be, msg.Flags))

	uuidBytes := utils.MustReturn(uuidSwitcheroo(msg.MessageId).MarshalBinary())
	utils.MustReturn(out.Write(uuidBytes))

	hasher := sha256.New()
	hasher.Write(msg.Payload)
	utils.MustReturn(out.Write(hasher.Sum(nil)))

	utils.Must(binary.Write(out, be, msg.PayloadType))
	utils.Must(binary.Write(out, be, msg.PayloadLength))

	utils.MustReturn(out.Write(msg.Payload))

	return out.Bytes()
}

func uuidSwitcheroo(in uuid.UUID) uuid.UUID {
	// The MGS protocol serializes UUIDs incorrectly. We need to swizzle some bytes,
	// so these two UUIDs are transformed into each other:
	// a3 de 28 2f - 47 8b - a6 e6 - 81 2e - f3 4f 87 bd 44 9e
	// 81 2e f3 4f - 87 bd - 44 9e - a3 de - 28 2f 47 8b a6 e6
	// Src pos:
	// 0  1  2  3    4  5    6  7    8  9    10 11 12 13 14 15
	// Dst pos:
	// 8  9  10 11   12 13   14 15   0  1    2  3  4   5  6 7
	var out [16]byte
	copy(out[0:], in[8:])
	copy(out[8:], in[0:])
	return out
}

func DeserializeMGSClientMessage(data []byte) (*ClientMessage, error) {
	reader := bytes.NewReader(data)
	be := binary.BigEndian

	res := ClientMessage{}

	err := binary.Read(reader, be, &res.HeaderLength)
	if err != nil {
		return nil, err
	}

	if res.HeaderLength == 0 {
		return nil, fmt.Errorf("`HeaderLength` cannot be zero")
	}

	if int64(len(data)) < int64(res.HeaderLength) {
		return nil, fmt.Errorf("`HeaderLength` is too large")
	}

	messageTypeBytes := [32]byte{}
	_, err = reader.Read(messageTypeBytes[:])
	if err != nil {
		return nil, err
	}
	res.MessageType = strings.Trim(string(messageTypeBytes[:]), " \x00") // Remove spaces and null characters

	err = binary.Read(reader, be, &res.SchemaVersion)
	if err != nil {
		return nil, err
	}

	err = binary.Read(reader, be, &res.CreatedDate)
	if err != nil {
		return nil, err
	}

	err = binary.Read(reader, be, &res.SequenceNumber)
	if err != nil {
		return nil, err
	}

	err = binary.Read(reader, be, &res.Flags)
	if err != nil {
		return nil, err
	}

	uuidBytes := [16]byte{}
	_, err = reader.Read(uuidBytes[:])
	if err != nil {
		return nil, err
	}
	res.MessageId = uuidSwitcheroo(uuid.Must(uuid.FromBytes(uuidBytes[:])))

	digestBytes := [32]byte{}
	_, err = reader.Read(digestBytes[:])
	if err != nil {
		return nil, err
	}
	res.PayloadDigest = digestBytes[:]

	err = binary.Read(reader, be, &res.PayloadType)
	if err != nil {
		return nil, err
	}

	err = binary.Read(reader, be, &res.PayloadLength)
	if err != nil {
		return nil, err
	}

	res.Payload = data[res.HeaderLength+4:]

	// MGS protocol has a bug, the start/pause publication messages have incorrect packet length and hash
	// Also, session termination message has incorrect payload length but a correct hash.
	// So it's best to just ignore the payload length field and rely on the hash sum to verify the
	// integrity.
	res.PayloadLength = uint32(len(res.Payload))

	workaround := StartPublicationMessage == res.MessageType || PausePublicationMessage == res.MessageType
	if !workaround && res.PayloadLength != 0 {
		sum := sha256.New()
		sum.Write(res.Payload)
		if !bytes.Equal(sum.Sum(nil), res.PayloadDigest) {
			return nil, fmt.Errorf("payload hash is not valid")
		}
	}

	return &res, nil
}
