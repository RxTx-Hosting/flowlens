package ebpf

type FlowKey struct {
	SrcIP   uint32
	DstPort uint16
	Proto   uint8
	_       uint8
}

type FlowInfo struct {
	Packets  uint64
	Bytes    uint64
	LastSeen uint64
}
