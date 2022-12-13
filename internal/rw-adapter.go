package internal

import (
	"github.com/Cyberax/gimlet/internal/utils"
	"github.com/Cyberax/gimlet/mgsproto"
	"io"
	"sync"
)

// Adapts the Pipeline+sender pair into an `io.ReadWriteCloser` to use with SMUX
type readWriter struct {
	mtx        sync.Mutex
	done       chan bool
	unconsumed []byte

	pipe   *utils.Pipeline[[]byte]
	sender func([]byte) error
}

var _ io.ReadWriteCloser = &readWriter{}

func NewReadWriter(pipe *utils.Pipeline[[]byte], sender func([]byte) error) io.ReadWriteCloser {
	return &readWriter{
		done:   make(chan bool),
		pipe:   pipe,
		sender: sender,
	}
}

func (r *readWriter) Read(p []byte) (n int, err error) {
	if len(r.unconsumed) > 0 {
		ln := len(r.unconsumed)
		if len(p) < ln {
			ln = len(p)
		}
		copy(p, r.unconsumed[0:ln])
		r.unconsumed = r.unconsumed[ln:]
		return ln, nil
	}
	select {
	case msg := <-r.pipe.ReadChan():
		if len(msg) <= len(p) {
			copy(p, msg)
			return len(msg), nil
		}
		copy(p, msg[0:len(p)])
		r.unconsumed = msg[len(p):]
		return len(p), nil
	case <-r.done:
		return 0, io.EOF
	}
}

func (r *readWriter) Write(p []byte) (n int, err error) {
	// Copy data to the stable storage, as writing is likely to be asynchronous
	dataLen := len(p)
	stable := make([]byte, dataLen)
	copy(stable, p)

	// Write data in max-sized chunks
	curPos := 0
	for curPos < dataLen {
		chunkSize := dataLen - curPos
		if chunkSize > mgsproto.MaxStreamingPayloadLength {
			chunkSize = mgsproto.MaxStreamingPayloadLength
		}

		err = r.sender(stable[curPos : curPos+chunkSize])
		if err != nil {
			return curPos, err
		}

		curPos += chunkSize
	}

	return dataLen, err
}

func (r *readWriter) Close() error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if !utils.IsChannelOpen(r.done) {
		return nil
	}
	close(r.done)
	return nil
}
