package utils

import (
	"container/list"
	"sync"
)

// Pipeline is an extension of Go channels that implements a channel with an infinite buffer.
// You need to call the `Start` method to start the channel, otherwise `Put` method will panic.
// It then behaves like a regular channel:
// `Put` method is used to submit messages, unlike regular channels it never blocks.
// `ReadChan` method can be used to get the read side of this pipeline.
// `Stop` method stops the pipeline. Unlike the regular channels, `Stop` method does not guarantee
// that all the pending messages are consumed.
type Pipeline[T any] struct {
	mtx      sync.Mutex
	started  bool
	done     bool
	doneChan chan bool

	hasElems *sync.Cond
	buffer   list.List
	stream   chan T
}

const PipelineBufSize = 4

// NewPipeline creates a new pipeline
func NewPipeline[T any]() *Pipeline[T] {
	p := &Pipeline[T]{
		doneChan: make(chan bool, PipelineBufSize),
		stream:   make(chan T),
	}
	p.hasElems = sync.NewCond(&p.mtx)
	return p
}

// ReadChan gets the read side of this pipeline
func (p *Pipeline[T]) ReadChan() <-chan T {
	return p.stream
}

// Start starts the pipeline. You need to call this method before you start enqueuing messages with `Put`
func (p *Pipeline[T]) Start() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	PanicIf(p.done, "pipeline has been stopped")
	p.started = true

	go func() {
		defer close(p.stream)
		for {
			if !p.sendToConsumer() {
				break
			}
		}
	}()
}

func (p *Pipeline[T]) sendToConsumer() bool {
	locked := true
	p.mtx.Lock()

	defer func() {
		if locked {
			p.mtx.Unlock()
		}
	}()

	for {
		if p.done {
			return false
		}
		if p.buffer.Len() != 0 {
			break
		}
		p.hasElems.Wait()
	}

	front := p.buffer.Front()
	p.buffer.Remove(front)

	// Release the lock, so that writers can continue publishing new items
	locked = false
	p.mtx.Unlock()

	// This can block. So this code runs outside the locked area
	select {
	case <-p.doneChan:
	case p.stream <- front.Value.(T):
	}

	return true
}

// Stop stops the pipeline. It does NOT guarantee that all the queued messages are consumed before
// the pipeline is stopped.
func (p *Pipeline[T]) Stop() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	PanicIf(!p.started, "pipeline has not been started")

	if p.done {
		return
	}
	p.done = true
	close(p.doneChan)
	p.hasElems.Signal()
}

// Put enqueues the message into the pipeline. Unlike the regular channel write, it's a simple
// no-op if the pipeline has been stopped already
func (p *Pipeline[T]) Put(elem T) {
	p.put(elem, false)
}

// PutFront enqueues the message into the front of the pipeline
func (p *Pipeline[T]) PutFront(elem T) {
	p.put(elem, true)
}

func (p *Pipeline[T]) put(elem T, addToFront bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.done {
		return
	}
	PanicIf(!p.started, "channel is not yet started")

	if addToFront {
		p.buffer.PushFront(elem)
	} else {
		p.buffer.PushBack(elem)
	}
	p.hasElems.Signal()
}

// Len returns the number of pending messages in the queue
func (p *Pipeline[T]) Len() int {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.done {
		return 0
	}

	return p.buffer.Len() + len(p.stream)
}
