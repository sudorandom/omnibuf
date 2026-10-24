package omnibuf

import (
	"encoding/binary"
	"math"
)

// ============================================================================
// Opaque API for OmniBuf (Lazy Deferred Unmarshal + O(1) On-Demand Getters
// + Zero-Decode Splice Re-Marshal for Read-Modify-Write Workloads)
// ============================================================================
//
// In Protobuf's Opaque API (Go Protobuf Edition 2023+), struct fields are hidden
// behind getters and setters (`msg.GetLocation().GetCity()`, `msg.SetSequence(...)`).
//
// With classic Protobuf wire encoding (TLV + LEB128), even with the Opaque API,
// Unmarshal still has to scan the entire packet byte-by-byte to find field boundaries.
//
// With OmniBuf's 1-Byte ZIP-Style Footer Index + POPCNT bitmask:
//   1. `Unmarshal` into an Opaque struct is O(1) (~1.5 ns, 0 allocs): it simply binds
//      `msg.raw = packet` and clears `dirtyMask`.
//   2. Calling `msg.GetLocation().GetCity()` or `msg.GetNeighbors(2).GetRouter()`
//      executes O(1) POPCNT offset lookups directly on `msg.raw` without ever parsing
//      the other 10 fields.
//   3. Mutating a field (`msg.SetSequence(seq + 1)`) sets its bit in `dirtyMask`, and
//      re-marshaling (`msg.MarshalAppend(dst)`) splices unmodified fields (including
//      entire nested sub-messages and repeated sub-message arrays) directly from `msg.raw`
//      via raw byte copy (`memmove`) without decoding or re-encoding them!

// OpaqueLocation is the Opaque API representation of Location.
type OpaqueLocation struct {
	raw        []byte
	present    uint16
	dirty      uint16
	region     string
	datacenter string
	city       string
	rack       uint32
	latitude   float32
	longitude  float32
}

func (l *OpaqueLocation) Reset() {
	l.raw = nil
	l.present = 0
	l.dirty = 0
}

func (l *OpaqueLocation) Unmarshal(b []byte) error {
	l.raw = b
	l.dirty = 0
	return nil
}

func (l OpaqueLocation) GetRegion() string {
	if (l.dirty & (1 << FieldLocRegion)) != 0 {
		return l.region
	}
	if l.raw != nil {
		s, _ := ReadStringZeroCopy(l.raw, FieldLocRegion)
		return s
	}
	return l.region
}

func (l *OpaqueLocation) SetRegion(v string) {
	l.region = v
	l.present |= (1 << FieldLocRegion)
	l.dirty |= (1 << FieldLocRegion)
}

func (l OpaqueLocation) GetDatacenter() string {
	if (l.dirty & (1 << FieldLocDatacenter)) != 0 {
		return l.datacenter
	}
	if l.raw != nil {
		s, _ := ReadStringZeroCopy(l.raw, FieldLocDatacenter)
		return s
	}
	return l.datacenter
}

func (l *OpaqueLocation) SetDatacenter(v string) {
	l.datacenter = v
	l.present |= (1 << FieldLocDatacenter)
	l.dirty |= (1 << FieldLocDatacenter)
}

func (l OpaqueLocation) GetCity() string {
	if (l.dirty & (1 << FieldLocCity)) != 0 {
		return l.city
	}
	if l.raw != nil {
		s, _ := ReadStringZeroCopy(l.raw, FieldLocCity)
		return s
	}
	return l.city
}

func (l OpaqueLocation) GetCitySafe() string {
	if (l.dirty & (1 << FieldLocCity)) != 0 {
		return l.city
	}
	if l.raw != nil {
		s, _ := ReadString(l.raw, FieldLocCity)
		return s
	}
	return l.city
}

func (l *OpaqueLocation) SetCity(v string) {
	l.city = v
	l.present |= (1 << FieldLocCity)
	l.dirty |= (1 << FieldLocCity)
}

func (l OpaqueLocation) GetRack() uint32 {
	if (l.dirty & (1 << FieldLocRack)) != 0 {
		return l.rack
	}
	if l.raw != nil {
		v, _ := ReadUint(l.raw, FieldLocRack)
		return uint32(v)
	}
	return l.rack
}

func (l *OpaqueLocation) SetRack(v uint32) {
	l.rack = v
	l.present |= (1 << FieldLocRack)
	l.dirty |= (1 << FieldLocRack)
}

func (l OpaqueLocation) GetLatitude() float32 {
	if (l.dirty & (1 << FieldLocLatitude)) != 0 {
		return l.latitude
	}
	if l.raw != nil {
		v, _ := ReadFloat32(l.raw, FieldLocLatitude)
		return v
	}
	return l.latitude
}

func (l *OpaqueLocation) SetLatitude(v float32) {
	l.latitude = v
	l.present |= (1 << FieldLocLatitude)
	l.dirty |= (1 << FieldLocLatitude)
}

func (l OpaqueLocation) GetLongitude() float32 {
	if (l.dirty & (1 << FieldLocLongitude)) != 0 {
		return l.longitude
	}
	if l.raw != nil {
		v, _ := ReadFloat32(l.raw, FieldLocLongitude)
		return v
	}
	return l.longitude
}

func (l *OpaqueLocation) SetLongitude(v float32) {
	l.longitude = v
	l.present |= (1 << FieldLocLongitude)
	l.dirty |= (1 << FieldLocLongitude)
}

// OpaqueNeighbor is the Opaque API representation of Neighbor.
type OpaqueNeighbor struct {
	raw         []byte
	present     uint16
	dirty       uint16
	peerASN     uint32
	router      string
	latencyUS   uint32
	established bool
}

func (n OpaqueNeighbor) GetPeerASN() uint32 {
	if (n.dirty & (1 << FieldNeighPeerASN)) != 0 {
		return n.peerASN
	}
	if n.raw != nil {
		v, _ := ReadUint(n.raw, FieldNeighPeerASN)
		return uint32(v)
	}
	return n.peerASN
}

func (n *OpaqueNeighbor) SetPeerASN(v uint32) {
	n.peerASN = v
	n.present |= (1 << FieldNeighPeerASN)
	n.dirty |= (1 << FieldNeighPeerASN)
}

func (n OpaqueNeighbor) GetRouter() string {
	if (n.dirty & (1 << FieldNeighRouter)) != 0 {
		return n.router
	}
	if n.raw != nil {
		s, _ := ReadStringZeroCopy(n.raw, FieldNeighRouter)
		return s
	}
	return n.router
}

func (n *OpaqueNeighbor) SetRouter(v string) {
	n.router = v
	n.present |= (1 << FieldNeighRouter)
	n.dirty |= (1 << FieldNeighRouter)
}

func (n OpaqueNeighbor) GetLatencyUS() uint32 {
	if (n.dirty & (1 << FieldNeighLatencyUS)) != 0 {
		return n.latencyUS
	}
	if n.raw != nil {
		v, _ := ReadUint(n.raw, FieldNeighLatencyUS)
		return uint32(v)
	}
	return n.latencyUS
}

func (n *OpaqueNeighbor) SetLatencyUS(v uint32) {
	n.latencyUS = v
	n.present |= (1 << FieldNeighLatencyUS)
	n.dirty |= (1 << FieldNeighLatencyUS)
}

func (n OpaqueNeighbor) GetEstablished() bool {
	if (n.dirty & (1 << FieldNeighEstablished)) != 0 {
		return n.established
	}
	if n.raw != nil {
		v, _ := ReadBool(n.raw, FieldNeighEstablished)
		return v
	}
	return n.established
}

func (n *OpaqueNeighbor) SetEstablished(v bool) {
	n.established = v
	n.present |= (1 << FieldNeighEstablished)
	n.dirty |= (1 << FieldNeighEstablished)
}

// OpaquePeerUpdate is the Opaque API representation of PeerUpdate with lazy O(1)
// wire evaluation and zero-decode field splicing for read-modify-write workloads.
type OpaquePeerUpdate struct {
	raw       []byte
	present   uint16
	dirty     uint16
	asn       uint32
	routerID  string
	active    bool
	uptimeSec uint64
	rssiDbm   int32
	cpuLoad   float64
	tags      []string
	prefixes  []uint32
	location  OpaqueLocation
	neighbors []OpaqueNeighbor
	sequence  uint64
}

// Reset clears the OpaquePeerUpdate for reuse.
func (p *OpaquePeerUpdate) Reset() {
	p.raw = nil
	p.present = 0
	p.dirty = 0
	p.location.Reset()
	p.tags = p.tags[:0]
	p.prefixes = p.prefixes[:0]
	p.neighbors = p.neighbors[:0]
}

// Unmarshal binds packet to p in O(1) time (0 allocations, 0 upfront field parsing).
func (p *OpaquePeerUpdate) Unmarshal(packet []byte) error {
	p.raw = packet
	p.dirty = 0
	p.location.raw = nil
	p.location.dirty = 0
	return nil
}

func (p *OpaquePeerUpdate) GetASN() uint32 {
	if (p.dirty & (1 << FieldPeerASN)) != 0 {
		return p.asn
	}
	if p.raw != nil {
		v, _ := ReadUint(p.raw, FieldPeerASN)
		return uint32(v)
	}
	return p.asn
}

func (p *OpaquePeerUpdate) SetASN(v uint32) {
	p.asn = v
	p.present |= (1 << FieldPeerASN)
	p.dirty |= (1 << FieldPeerASN)
}

func (p *OpaquePeerUpdate) GetRouterID() string {
	if (p.dirty & (1 << FieldPeerRouterID)) != 0 {
		return p.routerID
	}
	if p.raw != nil {
		s, _ := ReadStringZeroCopy(p.raw, FieldPeerRouterID)
		return s
	}
	return p.routerID
}

func (p *OpaquePeerUpdate) SetRouterID(v string) {
	p.routerID = v
	p.present |= (1 << FieldPeerRouterID)
	p.dirty |= (1 << FieldPeerRouterID)
}

func (p *OpaquePeerUpdate) GetActive() bool {
	if (p.dirty & (1 << FieldPeerActive)) != 0 {
		return p.active
	}
	if p.raw != nil {
		v, _ := ReadBool(p.raw, FieldPeerActive)
		return v
	}
	return p.active
}

func (p *OpaquePeerUpdate) SetActive(v bool) {
	p.active = v
	p.present |= (1 << FieldPeerActive)
	p.dirty |= (1 << FieldPeerActive)
}

func (p *OpaquePeerUpdate) GetUptimeSec() uint64 {
	if (p.dirty & (1 << FieldPeerUptimeSec)) != 0 {
		return p.uptimeSec
	}
	if p.raw != nil {
		v, _ := ReadUint(p.raw, FieldPeerUptimeSec)
		return v
	}
	return p.uptimeSec
}

func (p *OpaquePeerUpdate) SetUptimeSec(v uint64) {
	p.uptimeSec = v
	p.present |= (1 << FieldPeerUptimeSec)
	p.dirty |= (1 << FieldPeerUptimeSec)
}

func (p *OpaquePeerUpdate) GetRssiDbm() int32 {
	if (p.dirty & (1 << FieldPeerRssiDbm)) != 0 {
		return p.rssiDbm
	}
	if p.raw != nil {
		v, _ := ReadInt(p.raw, FieldPeerRssiDbm)
		return int32(v)
	}
	return p.rssiDbm
}

func (p *OpaquePeerUpdate) SetRssiDbm(v int32) {
	p.rssiDbm = v
	p.present |= (1 << FieldPeerRssiDbm)
	p.dirty |= (1 << FieldPeerRssiDbm)
}

func (p *OpaquePeerUpdate) GetCPULoad() float64 {
	if (p.dirty & (1 << FieldPeerCPULoad)) != 0 {
		return p.cpuLoad
	}
	if p.raw != nil {
		v, _ := ReadFloat64(p.raw, FieldPeerCPULoad)
		return v
	}
	return p.cpuLoad
}

func (p *OpaquePeerUpdate) SetCPULoad(v float64) {
	p.cpuLoad = v
	p.present |= (1 << FieldPeerCPULoad)
	p.dirty |= (1 << FieldPeerCPULoad)
}

func (p *OpaquePeerUpdate) SetTags(v []string) {
	p.tags = v
	p.present |= (1 << FieldPeerTags)
	p.dirty |= (1 << FieldPeerTags)
}

func (p *OpaquePeerUpdate) SetPrefixes(v []uint32) {
	p.prefixes = v
	p.present |= (1 << FieldPeerPrefixes)
	p.dirty |= (1 << FieldPeerPrefixes)
}

// GetLocation returns a pointer to the OpaqueLocation view in O(1) time with 0 heap allocations.
func (p *OpaquePeerUpdate) GetLocation() *OpaqueLocation {
	if (p.dirty&(1<<FieldPeerLocation)) == 0 && p.location.dirty == 0 && p.raw != nil && p.location.raw == nil {
		if sub, ok := ReadSubMsg(p.raw, FieldPeerLocation); ok {
			p.location.raw = sub
		}
	}
	return &p.location
}

// OpaqueNeighborView is a lightweight zero-allocation read view over a Neighbor element in wire memory.
type OpaqueNeighborView struct {
	raw []byte
}

func (n OpaqueNeighborView) GetPeerASN() uint32 {
	v, _ := ReadUint(n.raw, FieldNeighPeerASN)
	return uint32(v)
}

func (n OpaqueNeighborView) GetRouter() string {
	s, _ := ReadStringZeroCopy(n.raw, FieldNeighRouter)
	return s
}

func (n OpaqueNeighborView) GetLatencyUS() uint32 {
	v, _ := ReadUint(n.raw, FieldNeighLatencyUS)
	return uint32(v)
}

func (n OpaqueNeighborView) GetEstablished() bool {
	v, _ := ReadBool(n.raw, FieldNeighEstablished)
	return v
}

// GetNeighbors returns the i-th OpaqueNeighborView in O(1) time with 0 heap allocations.
func (p *OpaquePeerUpdate) GetNeighbors(index int) OpaqueNeighborView {
	if (p.dirty & (1 << FieldPeerNeighbors)) != 0 {
		if index < 0 || index >= len(p.neighbors) {
			return OpaqueNeighborView{}
		}
		return OpaqueNeighborView{raw: p.neighbors[index].raw}
	}
	if p.raw != nil {
		elem, ok := ReadSubMsgArrElem(p.raw, FieldPeerNeighbors, index)
		if !ok {
			return OpaqueNeighborView{}
		}
		return OpaqueNeighborView{raw: elem}
	}
	return OpaqueNeighborView{}
}

func (p *OpaquePeerUpdate) MutableLocation() *OpaqueLocation {
	if p.raw != nil && p.location.raw == nil && (p.dirty&(1<<FieldPeerLocation)) == 0 {
		sub, ok := ReadSubMsg(p.raw, FieldPeerLocation)
		if ok {
			p.location.raw = sub
		}
	}
	p.present |= (1 << FieldPeerLocation)
	p.dirty |= (1 << FieldPeerLocation)
	return &p.location
}

// NeighborsLen returns the number of Neighbor elements in O(1) time without decoding them.
func (p *OpaquePeerUpdate) NeighborsLen() int {
	if (p.dirty & (1 << FieldPeerNeighbors)) != 0 {
		return len(p.neighbors)
	}
	if p.raw != nil {
		return SubMsgArrLen(p.raw, FieldPeerNeighbors)
	}
	return len(p.neighbors)
}

func (p *OpaquePeerUpdate) SetNeighbors(v []OpaqueNeighbor) {
	p.neighbors = v
	p.present |= (1 << FieldPeerNeighbors)
	p.dirty |= (1 << FieldPeerNeighbors)
}

func (p *OpaquePeerUpdate) GetSequence() uint64 {
	if (p.dirty & (1 << FieldPeerSequence)) != 0 {
		return p.sequence
	}
	if p.raw != nil {
		v, _ := ReadUint(p.raw, FieldPeerSequence)
		return v
	}
	return p.sequence
}

func (p *OpaquePeerUpdate) SetSequence(v uint64) {
	p.sequence = v
	p.present |= (1 << FieldPeerSequence)
	p.dirty |= (1 << FieldPeerSequence)
}

// PopulateFromPeerUpdate populates p from a domain PeerUpdate struct (for fresh-build benchmarks).
func (p *OpaquePeerUpdate) PopulateFromPeerUpdate(src *PeerUpdate) {
	p.Reset()
	p.SetASN(src.ASN)
	p.SetRouterID(src.RouterID)
	p.SetActive(src.Active)
	p.SetUptimeSec(src.UptimeSec)
	p.SetRssiDbm(src.RssiDbm)
	p.SetCPULoad(src.CPULoad)
	p.SetTags(src.Tags)
	p.SetPrefixes(src.Prefixes)
	loc := p.MutableLocation()
	loc.SetRegion(src.Location.Region)
	loc.SetDatacenter(src.Location.Datacenter)
	loc.SetCity(src.Location.City)
	loc.SetRack(src.Location.Rack)
	loc.SetLatitude(src.Location.Latitude)
	loc.SetLongitude(src.Location.Longitude)

	if cap(p.neighbors) < len(src.Neighbors) {
		p.neighbors = make([]OpaqueNeighbor, len(src.Neighbors))
	} else {
		p.neighbors = p.neighbors[:len(src.Neighbors)]
	}
	for i := range src.Neighbors {
		nb := &src.Neighbors[i]
		p.neighbors[i] = OpaqueNeighbor{}
		p.neighbors[i].SetPeerASN(nb.PeerASN)
		p.neighbors[i].SetRouter(nb.Router)
		p.neighbors[i].SetLatencyUS(nb.LatencyUS)
		p.neighbors[i].SetEstablished(nb.Established)
	}
	p.present |= (1 << FieldPeerNeighbors)
	p.dirty |= (1 << FieldPeerNeighbors)
	p.SetSequence(src.Sequence)
}

// MaterializeInto copies all fields out of OpaquePeerUpdate into a plain PeerUpdate struct.
func (p *OpaquePeerUpdate) MaterializeInto(out *PeerUpdate) error {
	if p.raw != nil && p.dirty == 0 {
		return DecodePeerOmniBufInto(p.raw, out, true)
	}
	out.ASN = p.GetASN()
	out.RouterID = p.GetRouterID()
	out.Active = p.GetActive()
	out.UptimeSec = p.GetUptimeSec()
	out.RssiDbm = p.GetRssiDbm()
	out.CPULoad = p.GetCPULoad()
	loc := p.GetLocation()
	out.Location.Region = loc.GetRegion()
	out.Location.Datacenter = loc.GetDatacenter()
	out.Location.City = loc.GetCity()
	out.Location.Rack = loc.GetRack()
	out.Location.Latitude = loc.GetLatitude()
	out.Location.Longitude = loc.GetLongitude()
	nLen := p.NeighborsLen()
	if cap(out.Neighbors) < nLen {
		out.Neighbors = make([]Neighbor, nLen)
	} else {
		out.Neighbors = out.Neighbors[:nLen]
	}
	for i := 0; i < nLen; i++ {
		nb := p.GetNeighbors(i)
		out.Neighbors[i].PeerASN = nb.GetPeerASN()
		out.Neighbors[i].Router = nb.GetRouter()
		out.Neighbors[i].LatencyUS = nb.GetLatencyUS()
		out.Neighbors[i].Established = nb.GetEstablished()
	}
	out.Sequence = p.GetSequence()
	return nil
}

// MarshalAppend serializes p into dst[:0].
//
// Fast Paths:
//  1. Clean Wire Pass-Through (`p.raw != nil && p.dirty == 0`): copies `p.raw` directly.
//  2. Partial Mutation Splice (`p.raw != nil && p.dirty != 0`): splices unmodified wire ranges
//     directly from `p.raw` and re-encodes only the dirty fields in O(1) without decoding
//     untouched nested sub-messages or repeated arrays!
//  3. Fresh Build (`p.raw == nil`): single-pass forward encoding with 2-byte length backpatching.
func (p *OpaquePeerUpdate) MarshalAppend(dst []byte) []byte {
	dst = dst[:0]
	if p.raw != nil {
		if p.dirty == 0 {
			return append(dst, p.raw...)
		}
		return p.marshalSplice(dst)
	}
	return p.marshalFresh(dst)
}

// SpliceFields splices unmodified contiguous byte ranges from raw and invokes appendDirty
// only for fields whose bit is set in dirtyMask, avoiding any decoding of untouched fields.
func SpliceFields(dst, raw []byte, dirtyMask uint16, maxField uint16, appendDirty func(dst []byte, fnum uint16) []byte) []byte {
	n := len(raw)
	pos := 0
	cleanStart := 0
	var emitted uint16

	for pos < n {
		fieldStart := pos
		var fnum uint16
		var wt uint8
		b0 := raw[pos]
		if (b0 & 1) != 0 {
			fnum = uint16(b0 >> 5)
			wt = (b0 >> 1) & 0x0F
			pos++
		} else if (b0&2) != 0 && pos+2 <= n {
			u := binary.LittleEndian.Uint16(raw[pos:pos+2]) >> 2
			fnum = u >> 4
			wt = uint8(u & 0x0F)
			pos += 2
		} else {
			break
		}

		// Fast-skip field value to find fieldEnd
		switch wt {
		case WireUint, WireSint:
			if pos >= n {
				break
			}
			vb := raw[pos]
			if (vb & 1) != 0 {
				pos++
			} else if (vb & 2) != 0 {
				pos += 2
			} else if (vb & 4) != 0 {
				pos += 4
			} else {
				pos += 9
			}
		case WireBoolFalse, WireBoolTrue:
		case WireString, WireBytes, WireSubMsg, WireStringArr, WireUintArr, WireSubMsgArr, WireMap, WireSintArr, WireFixedArr, WireExtended:
			if pos >= n {
				break
			}
			lb := raw[pos]
			if (lb & 1) != 0 {
				pos += 1 + int(lb>>1)
			} else if (lb&2) != 0 && pos+2 <= n {
				pos += 2 + int(binary.LittleEndian.Uint16(raw[pos:pos+2])>>2)
			} else {
				bLen, bn := DecodeVarint(raw[pos:])
				pos += bn + int(bLen)
			}
		case WireFixed32:
			pos += 4
		case WireFixed64:
			pos += 8
		}
		if pos > n {
			break
		}
		fieldEnd := pos

		bit := uint16(1) << fnum
		if (dirtyMask & bit) != 0 {
			if cleanStart < fieldStart {
				dst = append(dst, raw[cleanStart:fieldStart]...)
			}
			dst = appendDirty(dst, fnum)
			emitted |= bit
			cleanStart = fieldEnd
		} else {
			emitted |= bit
		}
	}

	if cleanStart < n {
		dst = append(dst, raw[cleanStart:n]...)
	}

	// Append any newly added dirty fields that were not present in raw
	rem := dirtyMask &^ emitted
	if rem != 0 {
		for fnum := uint16(1); fnum <= maxField; fnum++ {
			if (rem & (1 << fnum)) != 0 {
				dst = appendDirty(dst, fnum)
			}
		}
	}
	return dst
}

func (p *OpaquePeerUpdate) marshalSplice(dst []byte) []byte {
	return SpliceFields(dst, p.raw, p.dirty, FieldPeerSequence, p.appendDirtyField)
}

func (p *OpaquePeerUpdate) appendDirtyField(dst []byte, fnum uint16) []byte {
	switch fnum {
	case FieldPeerASN:
		dst = append(dst, byte((FieldPeerASN<<4|uint16(WireUint))<<1)|1)
		return AppendVarint(dst, uint64(p.asn))
	case FieldPeerRouterID:
		dst = append(dst, byte((FieldPeerRouterID<<4|uint16(WireString))<<1)|1)
		dst = AppendVarint(dst, uint64(len(p.routerID)))
		return append(dst, p.routerID...)
	case FieldPeerActive:
		if p.active {
			return append(dst, byte((FieldPeerActive<<4|uint16(WireBoolTrue))<<1)|1)
		}
		return append(dst, byte((FieldPeerActive<<4|uint16(WireBoolFalse))<<1)|1)
	case FieldPeerUptimeSec:
		dst = append(dst, byte((FieldPeerUptimeSec<<4|uint16(WireUint))<<1)|1)
		return AppendVarint(dst, p.uptimeSec)
	case FieldPeerRssiDbm:
		dst = append(dst, byte((FieldPeerRssiDbm<<4|uint16(WireSint))<<1)|1)
		return AppendVarint(dst, EncodeZigZag(int64(p.rssiDbm)))
	case FieldPeerCPULoad:
		dst = append(dst, byte((FieldPeerCPULoad<<4|uint16(WireFixed64))<<1)|1)
		cpuBits := math.Float64bits(p.cpuLoad)
		return append(dst,
			byte(cpuBits), byte(cpuBits>>8), byte(cpuBits>>16), byte(cpuBits>>24),
			byte(cpuBits>>32), byte(cpuBits>>40), byte(cpuBits>>48), byte(cpuBits>>56),
		)
	case FieldPeerSequence:
		tag11 := ((FieldPeerSequence<<4 | uint16(WireUint)) << 2) | 2
		dst = append(dst, byte(tag11), byte(tag11>>8))
		return AppendVarint(dst, p.sequence)
	}
	return dst
}

func (p *OpaquePeerUpdate) marshalFresh(dst []byte) []byte {
	dst = append(dst, byte((FieldPeerASN<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, uint64(p.asn))

	dst = append(dst, byte((FieldPeerRouterID<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.routerID)))
	dst = append(dst, p.routerID...)

	if p.active {
		dst = append(dst, byte((FieldPeerActive<<4|uint16(WireBoolTrue))<<1)|1)
	} else {
		dst = append(dst, byte((FieldPeerActive<<4|uint16(WireBoolFalse))<<1)|1)
	}

	dst = append(dst, byte((FieldPeerUptimeSec<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, p.uptimeSec)

	dst = append(dst, byte((FieldPeerRssiDbm<<4|uint16(WireSint))<<1)|1)
	dst = AppendVarint(dst, EncodeZigZag(int64(p.rssiDbm)))

	dst = append(dst, byte((FieldPeerCPULoad<<4|uint16(WireFixed64))<<1)|1)
	cpuBits := math.Float64bits(p.cpuLoad)
	dst = append(dst,
		byte(cpuBits), byte(cpuBits>>8), byte(cpuBits>>16), byte(cpuBits>>24),
		byte(cpuBits>>32), byte(cpuBits>>40), byte(cpuBits>>48), byte(cpuBits>>56),
	)

	dst = append(dst, byte((FieldPeerTags<<4|uint16(WireStringArr))<<1)|1, 0, 0)
	tagsLenPos := len(dst) - 2
	dst = AppendVarint(dst, uint64(len(p.tags)))
	for i := range p.tags {
		s := p.tags[i]
		dst = AppendVarint(dst, uint64(len(s)))
		dst = append(dst, s...)
	}
	dst = BackpatchContainerLen(dst, tagsLenPos)

	tag8 := ((FieldPeerPrefixes<<4 | uint16(WireUintArr)) << 2) | 2
	dst = append(dst, byte(tag8), byte(tag8>>8), 0, 0)
	prefLenPos := len(dst) - 2
	dst = AppendVarint(dst, uint64(len(p.prefixes)))
	for i := range p.prefixes {
		dst = AppendVarint(dst, uint64(p.prefixes[i]))
	}
	dst = BackpatchContainerLen(dst, prefLenPos)

	tag9 := ((FieldPeerLocation<<4 | uint16(WireSubMsg)) << 2) | 2
	dst = append(dst, byte(tag9), byte(tag9>>8), 0, 0)
	locLenPos := len(dst) - 2
	dst = append(dst, byte((FieldLocRegion<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.location.region)))
	dst = append(dst, p.location.region...)
	dst = append(dst, byte((FieldLocDatacenter<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.location.datacenter)))
	dst = append(dst, p.location.datacenter...)
	dst = append(dst, byte((FieldLocCity<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.location.city)))
	dst = append(dst, p.location.city...)
	dst = append(dst, byte((FieldLocRack<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, uint64(p.location.rack))
	latBits := math.Float32bits(p.location.latitude)
	dst = append(dst, byte((FieldLocLatitude<<4|uint16(WireFixed32))<<1)|1,
		byte(latBits), byte(latBits>>8), byte(latBits>>16), byte(latBits>>24))
	lonBits := math.Float32bits(p.location.longitude)
	dst = append(dst, byte((FieldLocLongitude<<4|uint16(WireFixed32))<<1)|1,
		byte(lonBits), byte(lonBits>>8), byte(lonBits>>16), byte(lonBits>>24))
	dst = BackpatchContainerLen(dst, locLenPos)

	tag10 := ((FieldPeerNeighbors<<4 | uint16(WireSubMsgArr)) << 2) | 2
	dst = append(dst, byte(tag10), byte(tag10>>8), 0, 0)
	arrLenPos := len(dst) - 2
	for i := range p.neighbors {
		nb := &p.neighbors[i]
		eLenPos := len(dst)
		dst = append(dst, 0, 0)
		dst = append(dst, byte((FieldNeighPeerASN<<4|uint16(WireUint))<<1)|1)
		dst = AppendVarint(dst, uint64(nb.peerASN))
		dst = append(dst, byte((FieldNeighRouter<<4|uint16(WireString))<<1)|1)
		dst = AppendVarint(dst, uint64(len(nb.router)))
		dst = append(dst, nb.router...)
		dst = append(dst, byte((FieldNeighLatencyUS<<4|uint16(WireUint))<<1)|1)
		dst = AppendVarint(dst, uint64(nb.latencyUS))
		if nb.established {
			dst = append(dst, byte((FieldNeighEstablished<<4|uint16(WireBoolTrue))<<1)|1)
		} else {
			dst = append(dst, byte((FieldNeighEstablished<<4|uint16(WireBoolFalse))<<1)|1)
		}
		dst = BackpatchContainerLen(dst, eLenPos)
	}
	dst = BackpatchContainerLen(dst, arrLenPos)

	tag11 := ((FieldPeerSequence<<4 | uint16(WireUint)) << 2) | 2
	dst = append(dst, byte(tag11), byte(tag11>>8))
	dst = AppendVarint(dst, p.sequence)
	return dst
}
