package internal

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/Cyberax/gimlet/internal/utils"
	"github.com/Cyberax/gimlet/log"
	"github.com/Cyberax/gimlet/mgsproto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const MaxResendNum = 1000
const PingTimeInterval = 30 * time.Second

var ErrTooManyRetries = fmt.Errorf("too many unacknowledged retries")

type outgoingMessage struct {
	// Doubly-linked list!
	next, prev *outgoingMessage

	sentAtTimeMs  atomic.Int64
	resendAttempt atomic.Uint32
	acknowledged  atomic.Bool

	isPriority      bool
	isDirectMessage bool

	sequenceId      int64
	data            []byte
	dataLenForStats uint64

	sourceMsg *mgsproto.ClientMessage
}

type Sender struct {
	conn   WsConn
	logger *log.GimletLogger
	stats  *log.Stats

	maxPps          uint64
	resendTimeout   time.Duration
	done            chan bool
	onErrorCallback ErrorCallbackFunc

	mtx        sync.Mutex
	sequenceId int64

	normalOutbound   *utils.Pipeline[*outgoingMessage]
	priorityOutbound *utils.Pipeline[*outgoingMessage]

	packetQueueHead *outgoingMessage
	packetQueueTail *outgoingMessage
	seqIdMap        map[int64]*outgoingMessage

	timer func() time.Time
}

func NewSender(conn WsConn, onErrorCallback ErrorCallbackFunc, maxPps uint64, resendTimeout time.Duration,
	log *log.GimletLogger, stats *log.Stats) *Sender {

	if onErrorCallback == nil {
		onErrorCallback = func(err error) {}
	}

	res := &Sender{
		logger:        log,
		stats:         stats,
		maxPps:        maxPps,
		resendTimeout: resendTimeout,

		done:             make(chan bool, 1),
		conn:             conn,
		onErrorCallback:  onErrorCallback,
		normalOutbound:   utils.NewPipeline[*outgoingMessage](),
		priorityOutbound: utils.NewPipeline[*outgoingMessage](),
		seqIdMap:         make(map[int64]*outgoingMessage),
		timer:            time.Now,
	}

	res.priorityOutbound.Start()
	res.normalOutbound.Start()
	go res.runSenderLoop()
	go res.runResendLoop()
	return res
}

// Shutdown shuts down the sender. Intra-class callers needs to
// make sure it's not called while `Sender.mtx` is held.
func (s *Sender) Shutdown() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if !utils.IsChannelOpen(s.done) {
		return
	}

	s.priorityOutbound.Stop()
	s.normalOutbound.Stop()
	close(s.done)
}

func (s *Sender) AckReceived(seqId int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	packet, ok := s.seqIdMap[seqId]
	if !ok {
		return
	}
	packet.acknowledged.Store(true)

	// Remove the packet from the ack scheduling queue (it's a doubly-linked list)
	if packet == s.packetQueueHead {
		s.packetQueueHead = packet.next
	}
	if packet == s.packetQueueTail {
		s.packetQueueTail = packet.prev
	}
	if packet.next != nil {
		packet.next.prev = packet.prev
	}
	if packet.prev != nil {
		packet.prev.next = packet.next
	}

	delete(s.seqIdMap, seqId)
	s.stats.UnackedSentPackets.Store(uint64(len(s.seqIdMap)))
}

func (s *Sender) EnqueueChannelOpen(token string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	reqId := uuid.New()
	clientId := uuid.New()

	openDataChannelInput := mgsproto.OpenDataChannelInput{
		MessageSchemaVersion: mgsproto.SchemaVersion,
		RequestId:            reqId.String(),
		TokenValue:           token,
		ClientId:             clientId.String(),
	}

	odciBytes, err := json.Marshal(&openDataChannelInput)
	utils.PanicIfErr(err)

	out := &outgoingMessage{
		isDirectMessage: true,
		data:            odciBytes,
	}
	s.priorityOutbound.Put(out)
	// This is a special message that doesn't need acks
}

func (s *Sender) EnqueueAck(messageType string, ackedMsgId uuid.UUID, ackedSequenceNumber int64) {
	ackContent := mgsproto.AcknowledgeContent{
		MessageType:         messageType,
		MessageId:           ackedMsgId.String(),
		SequenceNumber:      ackedSequenceNumber,
		IsSequentialMessage: true,
	}
	ackBytes, err := json.Marshal(ackContent)
	utils.PanicIfErr(err)

	messageId := uuid.New()

	s.mtx.Lock()
	defer s.mtx.Unlock()

	now := s.timer()

	clientMessage := mgsproto.ClientMessage{
		MessageType:    mgsproto.AcknowledgeMessage,
		SchemaVersion:  mgsproto.SchemaVersionNum,
		CreatedDate:    uint64(now.UnixMilli()),
		SequenceNumber: 0,
		Flags:          mgsproto.ClientMessageFlagSYN | mgsproto.ClientMessageFlagFIN,
		MessageId:      messageId,
		Payload:        ackBytes,
	}

	ackBytes = mgsproto.SerializeMGSClientMessage(&clientMessage)
	out := &outgoingMessage{
		data:       ackBytes,
		isPriority: true,
		sourceMsg:  &clientMessage,
	}
	s.priorityOutbound.Put(out)
	// No need to wait for acks, we just fire-and-forget OUR ack messages
}

func (s *Sender) SerializeAndEnqueueControl(msgType mgsproto.PayloadType, data any) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return s.enqueueMessage(msgType, dataBytes, true)
}

func (s *Sender) EnqueueData(data []byte) error {
	return s.enqueueMessage(mgsproto.Output, data, false)
}

func (s *Sender) enqueueMessage(msgType mgsproto.PayloadType, data []byte, prio bool) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if !utils.IsChannelOpen(s.done) {
		return io.ErrClosedPipe
	}

	now := s.timer()
	messageId := uuid.New()

	clientMessage := mgsproto.ClientMessage{
		MessageType:    mgsproto.InputStreamMessage,
		SchemaVersion:  mgsproto.SchemaVersionNum,
		CreatedDate:    uint64(now.UnixMilli()),
		SequenceNumber: s.sequenceId,
		Flags:          0,
		MessageId:      messageId,
		PayloadType:    msgType,
		Payload:        data,
		PayloadLength:  uint32(len(data)),
	}

	msgBytes := mgsproto.SerializeMGSClientMessage(&clientMessage)
	out := &outgoingMessage{
		sequenceId: s.sequenceId,
		data:       msgBytes,
		sourceMsg:  &clientMessage,
	}
	if msgType == mgsproto.Output {
		out.dataLenForStats = uint64(len(data))
	}

	// Make sure the packet is registered for the acks,
	// link in the packet to the end of the retry queue.
	if s.packetQueueTail == nil {
		s.packetQueueHead = out
		s.packetQueueTail = out
	} else {
		s.packetQueueTail.next = out
		out.prev = s.packetQueueTail
		s.packetQueueTail = out
	}

	s.seqIdMap[out.sequenceId] = out
	s.stats.UnackedSentPackets.Store(uint64(len(s.seqIdMap)))

	if prio {
		s.priorityOutbound.Put(out)
	} else {
		s.normalOutbound.Put(out)
	}

	s.sequenceId++
	return nil
}

func (s *Sender) EnqueueControlFlag(flagType mgsproto.FlagMessage) error {
	flagBuf := new(bytes.Buffer)
	_ = binary.Write(flagBuf, binary.BigEndian, flagType)
	return s.enqueueMessage(mgsproto.Flag, flagBuf.Bytes(), true)
}

func (s *Sender) runResendLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-s.done:
			break loop
		case <-ticker.C:
		}

		s.tryResending()
	}
}

func (s *Sender) tryResending() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	now := s.timer().UnixMilli()
	retryCutoff := now - s.resendTimeout.Milliseconds()

	for cur := s.packetQueueHead; cur != nil; cur = cur.next {
		utils.PanicIf(cur.acknowledged.Load(), "Message acknowledged and still in queue")

		sentAt := cur.sentAtTimeMs.Load()
		// Each time a message is re-sent (or enqueued for the first time), we move it to the
		// tail of the queue and clean its `sentAtTimeMs` field.
		// If we hit such a message here, then we've processed all messages that need retries.
		if sentAt == 0 {
			break
		}
		// The message is not yet eligible for re-sending, so any further messages are not
		// re-sendable either. Stop the loop.
		if sentAt > retryCutoff {
			break
		}

		// Resend the message, mark it as unsent and move it to the tail of the packet queue
		cur.sentAtTimeMs.Store(0)
		// We're always moving packets from the queue head to the tail
		utils.PanicIf(s.packetQueueHead != cur, "packet is not in the head")
		if cur.next != nil {
			s.packetQueueHead = cur.next
			cur.next.prev = nil
			cur.next = nil

			s.packetQueueTail.next = cur
			cur.prev = s.packetQueueTail
			s.packetQueueTail = cur
		}

		if cur.isPriority {
			s.priorityOutbound.Put(cur)
		} else {
			s.normalOutbound.Put(cur)
		}
	}
}

func (s *Sender) runSenderLoop() {
	go func() {
		select {
		case <-s.done:
		}
		//Cause the pending possibly blocked write to stop immediately
		_ = s.conn.SetWriteDeadline(time.Now())
	}()

	pinger := time.NewTicker(PingTimeInterval)
	defer pinger.Stop()

	p := newPacer(s.maxPps, s.maxPps)
	defer p.stop()

	// It's safe to call Shutdown() because no locks are held here
	defer s.Shutdown()

loop:
	for {
		// Pace the sending rate
		select {
		case <-p.permitChannel():
		case <-s.done:
			break loop
		}

		m, doContinue := s.tryDequeMessage(pinger)
		if !doContinue {
			break
		}
		if m != nil {
			s.send(m)
		}
	}
}

func (s *Sender) tryDequeMessage(pinger *time.Ticker) (*outgoingMessage, bool) {
	var msg *outgoingMessage

	// First try to read the priority message (non-blocking)
	select {
	case <-s.done:
		return nil, false
	case msg = <-s.priorityOutbound.ReadChan():
	default:
	}

	// Then try to dequeue message from both the priority and the normal queues
	if msg == nil {
		select {
		case <-s.done:
			return nil, false
		case msg = <-s.normalOutbound.ReadChan():
		case msg = <-s.priorityOutbound.ReadChan():
		case <-pinger.C:
			// A simple fire-and-forget ping message
			s.stats.NumPingsSent.Add(1)
			_ = s.conn.WriteMessage(websocket.PingMessage, []byte("keepalive"))
			return nil, true
		}
	}

	if msg == nil {
		return nil, false
	}

	return msg, true
}

func (s *Sender) send(m *outgoingMessage) {
	// Check if the message has been acknowledged while dwelling in the queue
	if m.acknowledged.Load() {
		return
	}
	defer m.sentAtTimeMs.Store(int64(uint64(s.timer().UnixMilli())))

	curAttempt := m.resendAttempt.Load()
	if curAttempt >= MaxResendNum {
		s.onErrorCallback(ErrTooManyRetries)
		return
	}

	if m.sourceMsg != nil {
		s.logger.LogSentPacket(m.sourceMsg, curAttempt)
	}

	var err error
	if m.isDirectMessage {
		err = s.conn.WriteMessage(websocket.TextMessage, m.data)
	} else {
		err = s.conn.WriteMessage(websocket.BinaryMessage, m.data)
	}

	if err != nil {
		if err == websocket.ErrCloseSent {
			s.onErrorCallback(io.EOF) // The normal closure
		} else {
			s.onErrorCallback(err)
		}
		return
	}

	s.stats.NumPacketsSent.Add(1)
	s.stats.DataBytesSent.Add(m.dataLenForStats)
	if curAttempt != 0 {
		s.stats.NumRetriesSent.Add(1)
	}
}
