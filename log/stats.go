package log

import (
	"encoding/json"
	"github.com/Cyberax/gimlet/internal/utils"
	"sync/atomic"
)

type Stats struct {
	OutOfOrderPacketsQueued atomic.Uint64
	OutOfOrderPackets       atomic.Uint64
	NumPacketsRead          atomic.Uint64
	ControlPacketsRead      atomic.Uint64
	DataBytesRead           atomic.Uint64
	NumAcksReceived         atomic.Uint64

	NumUnknownPackets atomic.Uint64
	NumFlags          atomic.Uint64

	NumPacketsSent     atomic.Uint64
	NumPingsSent       atomic.Uint64
	DataBytesSent      atomic.Uint64
	NumRetriesSent     atomic.Uint64
	UnackedSentPackets atomic.Uint64
}

func (s *Stats) String() string {
	vals := make(map[string]uint64)

	vals["OutOfOrderPacketsQueued"] = s.OutOfOrderPacketsQueued.Load()
	vals["OutOfOrderPackets"] = s.OutOfOrderPackets.Load()
	vals["NumPacketsRead"] = s.NumPacketsRead.Load()
	vals["ControlPacketsRead"] = s.ControlPacketsRead.Load()
	vals["DataBytesRead"] = s.DataBytesRead.Load()
	vals["NumAcksReceived"] = s.NumAcksReceived.Load()

	vals["NumUnknownPackets"] = s.NumUnknownPackets.Load()
	vals["NumFlags"] = s.NumFlags.Load()

	vals["NumPacketsSent"] = s.NumPacketsSent.Load()
	vals["NumPingsSent"] = s.NumPingsSent.Load()
	vals["DataBytesSent"] = s.DataBytesSent.Load()
	vals["NumRetriesSent"] = s.NumRetriesSent.Load()

	vals["UnackedSentPackets"] = s.UnackedSentPackets.Load()

	return string(utils.MustReturn(json.Marshal(vals)))
}
