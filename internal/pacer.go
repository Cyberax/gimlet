package internal

import (
	"github.com/Cyberax/gimlet/internal/utils"
	"time"
)

type pacer struct {
	done   chan bool
	tokens chan bool

	nanosPerToken   int64
	slackMicros     int64
	maxCapacity     int64
	lastReplenished time.Time
	timer           func() time.Time
}

func newPacer(replenishRatePerSecond uint64, maxCapacity uint64) *pacer {
	nanosPerToken := int64(1_000_000_000) / int64(replenishRatePerSecond)
	utils.PanicIf(nanosPerToken <= 1, "replenish rate is too high")

	timer := time.Now
	res := &pacer{
		done:            make(chan bool),
		tokens:          make(chan bool, maxCapacity),
		nanosPerToken:   nanosPerToken,
		maxCapacity:     int64(maxCapacity),
		lastReplenished: timer(),
		timer:           timer,
	}
	go res.tokenReplenishLoop()
	return res
}

func (p *pacer) stop() {
	close(p.done)
}

func (p *pacer) tokenReplenishLoop() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

loop:
	for {
		var now time.Time

		select {
		case now = <-t.C:
		case <-p.done:
			break loop
		}

		// How many permits do we have?
		nanos := now.Sub(p.lastReplenished).Nanoseconds()
		numTokens := nanos / p.nanosPerToken
		if numTokens <= 0 {
			continue
		}

		if numTokens > p.maxCapacity {
			numTokens = p.maxCapacity
		}

		p.lastReplenished = now
		// Send tokens until we reach the channel's capacity
	innerLoop:
		for i := int64(0); i < numTokens; i++ {
			select {
			case p.tokens <- true:
			default:
				break innerLoop
			}
		}
	}
}

func (p *pacer) permitChannel() <-chan bool {
	return p.tokens
}
