package internal

import (
	"fmt"
	"github.com/Cyberax/gimlet/internal/utils"
	"github.com/Cyberax/gimlet/log"
	"github.com/Cyberax/gimlet/mgsproto"
	"io"
	"sync"
	"time"
)

type Receiver struct {
	logger *log.GimletLogger
	stats  *log.Stats

	conn            WsConn
	done            chan bool
	onErrorCallback ErrorCallbackFunc
	acks            AckReceiver

	mtx                sync.Mutex
	expectedSequenceId int64
	outOfOrderMessages map[int64]*mgsproto.ClientMessage
	controlMsgQueue    *utils.Pipeline[*mgsproto.ClientMessage]
	dataQueue          *utils.Pipeline[[]byte]

	timer func() time.Time
}

func NewReceiver(conn WsConn, onErrorCallback ErrorCallbackFunc,
	acks AckReceiver, logger *log.GimletLogger, stats *log.Stats) *Receiver {

	res := &Receiver{
		logger:             logger,
		stats:              stats,
		conn:               conn,
		done:               make(chan bool),
		onErrorCallback:    onErrorCallback,
		acks:               acks,
		outOfOrderMessages: make(map[int64]*mgsproto.ClientMessage),
		controlMsgQueue:    utils.NewPipeline[*mgsproto.ClientMessage](),
		dataQueue:          utils.NewPipeline[[]byte](),
		timer:              time.Now,
	}

	res.controlMsgQueue.Start()
	res.dataQueue.Start()
	go res.runReader()

	return res
}

func (r *Receiver) ControlReaderQueue() *utils.Pipeline[*mgsproto.ClientMessage] {
	return r.controlMsgQueue
}

func (r *Receiver) DataReaderQueue() *utils.Pipeline[[]byte] {
	return r.dataQueue
}

func (r *Receiver) Shutdown() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if !utils.IsChannelOpen(r.done) {
		return
	}
	close(r.done)
	r.controlMsgQueue.Stop()
	r.dataQueue.Stop()
}

func (r *Receiver) runReader() {
	go func() {
		select {
		case <-r.done:
		}
		//Cause the pending possibly blocked read to stop immediately
		_ = r.conn.SetReadDeadline(time.Now())
	}()

	// It's safe to call r.Shutdown() since we're not holding any locks here
	defer r.Shutdown()
	for {
		if !r.runReaderIteration() {
			break
		}
	}
}

func (r *Receiver) runReaderIteration() bool {
	msgBytes, err := r.conn.ReadMessage()
	if err != nil {
		if err == io.EOF {
			r.onErrorCallback(io.EOF)
		} else {
			err := fmt.Errorf("failed to read the message: %w", err)
			r.onErrorCallback(err)
		}
		return false
	}

	msg, err := mgsproto.DeserializeMGSClientMessage(msgBytes)
	if err != nil {
		err := fmt.Errorf("failed to deserialize message: %w", err)
		r.onErrorCallback(err)
		return false
	}

	r.logger.LogRecvPacket(msg)
	r.processMessage(msg)

	return true
}

func (r *Receiver) processMessage(msg *mgsproto.ClientMessage) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.stats.NumPacketsRead.Add(1)

	// Treat anything but output_stream messages as control-plane data. These messages don't
	// have a correct SeqId and need to be processed separately. They also don't need acks.
	if msg.MessageType != mgsproto.OutputStreamMessage {
		if msg.MessageType == mgsproto.AcknowledgeMessage {
			// TODO: fastpath for acks?
			r.stats.NumAcksReceived.Add(1)
		}
		r.stats.ControlPacketsRead.Add(1)
		r.controlMsgQueue.Put(msg)
		return
	}

	// Send acks for output_stream messages
	// TODO: add CoDel-like moderating of the flow
	r.acks(msg.MessageType, msg.MessageId, msg.SequenceNumber) // Send the acknowledgement

	// A duplicate message. It happens.
	if msg.SequenceNumber < r.expectedSequenceId {
		r.stats.OutOfOrderPackets.Add(1)
		return
	}

	// Postpone the processing of an out-of-order message
	if msg.SequenceNumber > r.expectedSequenceId {
		r.stats.OutOfOrderPackets.Add(1)
		r.outOfOrderMessages[msg.SequenceNumber] = msg
		r.stats.OutOfOrderPacketsQueued.Store(uint64(len(r.outOfOrderMessages)))
		return
	}

	// We've received a message with expected sequence ID, so we can just immediately
	// send it to the consumer and increment the expected sequence number.
	if msg.PayloadType == mgsproto.Output {
		r.stats.DataBytesRead.Add(uint64(msg.PayloadLength))
		r.dataQueue.Put(msg.Payload)
	} else {
		r.stats.ControlPacketsRead.Add(1)
		r.controlMsgQueue.Put(msg)
	}
	r.expectedSequenceId++

	// Check if we now can also send other delayed messages
	for len(r.outOfOrderMessages) != 0 {
		msg, ok := r.outOfOrderMessages[r.expectedSequenceId]
		if ok {
			if msg.PayloadType == mgsproto.Output && msg.MessageType == mgsproto.OutputStreamMessage {
				// TODO: discard messages if the consumer can't keep up
				r.dataQueue.Put(msg.Payload)
			} else {
				r.controlMsgQueue.Put(msg)
			}
			delete(r.outOfOrderMessages, r.expectedSequenceId)
			r.expectedSequenceId++
		} else {
			break
		}
	}

	r.stats.OutOfOrderPacketsQueued.Store(uint64(len(r.outOfOrderMessages)))
}
