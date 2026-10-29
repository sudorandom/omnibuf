package omnibuf_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"unsafe"

	capnp "capnproto.org/go/capnp/v3"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/hamba/avro/v2"
	"github.com/hamba/avro/v2/ocf"
	. "github.com/sudorandom/omnibuf"
	. "github.com/sudorandom/omnibuf/wire"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/encoding/protowire"
)

// Canonical benchmark schema types matching proto/omnibuf/bench/peer.proto and testdata/schemas/peer.{fbs,capnp}.

type Location struct {
	Region     string  `omnibuf:"1,name=region" protobuf:"bytes,1,opt,name=region,proto3" json:"region" msgpack:"region" avro:"region"`
	Datacenter string  `omnibuf:"2,name=datacenter" protobuf:"bytes,2,opt,name=datacenter,proto3" json:"datacenter" msgpack:"datacenter" avro:"datacenter"`
	City       string  `omnibuf:"3,name=city" protobuf:"bytes,3,opt,name=city,proto3" json:"city" msgpack:"city" avro:"city"`
	Rack       uint32  `omnibuf:"4,name=rack" protobuf:"varint,4,opt,name=rack,proto3" json:"rack" msgpack:"rack" avro:"rack"`
	Latitude   float32 `omnibuf:"5,name=latitude" protobuf:"fixed32,5,opt,name=latitude,proto3" json:"latitude" msgpack:"latitude" avro:"latitude"`
	Longitude  float32 `omnibuf:"6,name=longitude" protobuf:"fixed32,6,opt,name=longitude,proto3" json:"longitude" msgpack:"longitude" avro:"longitude"`
}

type Neighbor struct {
	PeerASN     uint32 `omnibuf:"1,name=peer_asn" protobuf:"varint,1,opt,name=peer_asn,proto3" json:"peer_asn" msgpack:"peer_asn" avro:"peer_asn"`
	Router      string `omnibuf:"2,name=router" protobuf:"bytes,2,opt,name=router,proto3" json:"router" msgpack:"router" avro:"router"`
	LatencyUS   uint32 `omnibuf:"3,name=latency_us" protobuf:"varint,3,opt,name=latency_us,proto3" json:"latency_us" msgpack:"latency_us" avro:"latency_us"`
	Established bool   `omnibuf:"4,name=established" protobuf:"varint,4,opt,name=established,proto3" json:"established" msgpack:"established" avro:"established"`
}

type PeerUpdate struct {
	ASN       uint32     `omnibuf:"1,name=asn" protobuf:"varint,1,opt,name=asn,proto3" json:"asn" msgpack:"asn" avro:"asn"`
	RouterID  string     `omnibuf:"2,name=router_id" protobuf:"bytes,2,opt,name=router_id,proto3" json:"router_id" msgpack:"router_id" avro:"router_id"`
	Active    bool       `omnibuf:"3,name=active" protobuf:"varint,3,opt,name=active,proto3" json:"active" msgpack:"active" avro:"active"`
	UptimeSec uint64     `omnibuf:"4,name=uptime_sec" protobuf:"varint,4,opt,name=uptime_sec,proto3" json:"uptime_sec" msgpack:"uptime_sec" avro:"uptime_sec"`
	RssiDbm   int32      `omnibuf:"5,name=rssi_dbm" protobuf:"zigzag32,5,opt,name=rssi_dbm,proto3" json:"rssi_dbm" msgpack:"rssi_dbm" avro:"rssi_dbm"`
	CPULoad   float64    `omnibuf:"6,name=cpu_load" protobuf:"fixed64,6,opt,name=cpu_load,proto3" json:"cpu_load" msgpack:"cpu_load" avro:"cpu_load"`
	Tags      []string   `omnibuf:"7,name=tags" protobuf:"bytes,7,rep,name=tags,proto3" json:"tags" msgpack:"tags" avro:"tags"`
	Prefixes  []uint32   `omnibuf:"8,name=prefixes" protobuf:"varint,8,rep,packed,name=prefixes,proto3" json:"prefixes" msgpack:"prefixes" avro:"prefixes"`
	Location  Location   `omnibuf:"9,name=location" protobuf:"bytes,9,opt,name=location,proto3" json:"location" msgpack:"location" avro:"location"`
	Neighbors []Neighbor `omnibuf:"10,name=neighbors" protobuf:"bytes,10,rep,name=neighbors,proto3" json:"neighbors" msgpack:"neighbors" avro:"neighbors"`
	Sequence  uint64     `omnibuf:"11,name=sequence" protobuf:"varint,11,opt,name=sequence,proto3" json:"sequence" msgpack:"sequence" avro:"sequence"`
}

// Field numbers in PeerUpdate, Location, and Neighbor.
const (
	FieldPeerASN       uint16 = 1
	FieldPeerRouterID  uint16 = 2
	FieldPeerActive    uint16 = 3
	FieldPeerUptimeSec uint16 = 4
	FieldPeerRssiDbm   uint16 = 5
	FieldPeerCPULoad   uint16 = 6
	FieldPeerTags      uint16 = 7
	FieldPeerPrefixes  uint16 = 8
	FieldPeerLocation  uint16 = 9
	FieldPeerNeighbors uint16 = 10
	FieldPeerSequence  uint16 = 11

	FieldLocRegion     uint16 = 1
	FieldLocDatacenter uint16 = 2
	FieldLocCity       uint16 = 3
	FieldLocRack       uint16 = 4
	FieldLocLatitude   uint16 = 5
	FieldLocLongitude  uint16 = 6

	FieldNeighPeerASN     uint16 = 1
	FieldNeighRouter      uint16 = 2
	FieldNeighLatencyUS   uint16 = 3
	FieldNeighEstablished uint16 = 4
)

var peerSchemaMeta = &SchemaMeta{
	TypeName: "PeerUpdate",
	Fields: map[uint32]string{
		1:  "asn",
		2:  "router_id",
		3:  "active",
		4:  "uptime_sec",
		5:  "rssi_dbm",
		6:  "cpu_load",
		7:  "tags",
		8:  "prefixes",
		9:  "location",
		10: "neighbors",
		11: "sequence",
	},
	Children: map[uint32]*SchemaMeta{
		9: {
			TypeName: "Location",
			Fields: map[uint32]string{
				1: "region",
				2: "datacenter",
				3: "city",
				4: "rack",
				5: "latitude",
				6: "longitude",
			},
		},
		10: {
			TypeName: "Neighbor",
			Fields: map[uint32]string{
				1: "peer_asn",
				2: "router",
				3: "latency_us",
				4: "established",
			},
		},
	},
}

// SamplePeerUpdate returns a representative BGP/Telemetry PeerUpdate payload.
func SamplePeerUpdate() PeerUpdate {
	return PeerUpdate{
		ASN:       64512,
		RouterID:  "edge-rtr-fra01.core.backbone.net",
		Active:    true,
		UptimeSec: 1849203,
		RssiDbm:   -62,
		CPULoad:   0.42875,
		Tags:      []string{"tier1", "ebgp", "eu-central", "ixp-de-cix"},
		Prefixes:  []uint32{167772160, 172032000, 3232235520, 2886729728, 134744072, 134743040},
		Location: Location{
			Region:     "eu-central-1",
			Datacenter: "FRA-Equinix-FR5",
			City:       "Frankfurt",
			Rack:       42,
			Latitude:   50.1109,
			Longitude:  8.6821,
		},
		Neighbors: []Neighbor{
			{PeerASN: 3356, Router: "ae0.fra1.level3.net", LatencyUS: 850, Established: true},
			{PeerASN: 1299, Router: "ffm-b5-link.telia.net", LatencyUS: 920, Established: true},
			{PeerASN: 174, Router: "be2040.ccr41.fra03.atlas.cogentco.com", LatencyUS: 1120, Established: true},
			{PeerASN: 2914, Router: "ae-12.r24.frnkge08.de.bb.gin.ntt.net", LatencyUS: 980, Established: false},
		},
		Sequence: 9876543210,
	}
}

// ============================================================================
// 1. OmniBuf Encoder & Decoder
// ============================================================================

func growSlice[T any](dst []T, add int) ([]T, int) {
	start := len(dst)
	total := start + add
	if cap(dst) < total || dst == nil {
		next := make([]T, total)
		copy(next, dst)
		return next, start
	}
	return dst[:total], start
}

func sliceForDecode[T any](p *[]T, alreadySeen bool) []T {
	sl := *p
	if !alreadySeen && sl != nil {
		return sl[:0]
	}
	return sl
}

var peerSchemaMetaWire = peerSchemaMeta.AppendField0(nil, true)

// EncodePeerOmniBuf encodes PeerUpdate into enc in shared-schema mode (or self-describing mode if withField0 is true).
func EncodePeerOmniBuf(enc *Encoder, p *PeerUpdate, withField0 bool) []byte {
	enc.Reset()
	if withField0 {
		enc.WriteRaw(peerSchemaMetaWire)
	}
	enc.WriteUint(FieldPeerASN, uint64(p.ASN))
	enc.WriteString(FieldPeerRouterID, p.RouterID)
	enc.WriteBool(FieldPeerActive, p.Active)
	enc.WriteUint(FieldPeerUptimeSec, p.UptimeSec)
	enc.WriteInt(FieldPeerRssiDbm, int64(p.RssiDbm))
	enc.WriteFloat64(FieldPeerCPULoad, p.CPULoad)
	enc.WriteStringArr(FieldPeerTags, p.Tags)
	enc.WriteUint32Arr(FieldPeerPrefixes, p.Prefixes)

	// Nested sub-message: Location (single-pass 2B backpatched)
	enc.BeginSubMsg(FieldPeerLocation)
	enc.WriteString(FieldLocRegion, p.Location.Region)
	enc.WriteString(FieldLocDatacenter, p.Location.Datacenter)
	enc.WriteString(FieldLocCity, p.Location.City)
	enc.WriteUint(FieldLocRack, uint64(p.Location.Rack))
	enc.WriteFloat32(FieldLocLatitude, p.Location.Latitude)
	enc.WriteFloat32(FieldLocLongitude, p.Location.Longitude)
	enc.EndSubMsg()

	// Indexed sub-message array: Neighbors (WireSubMsgArr)
	enc.BeginSubMsgArr(FieldPeerNeighbors)
	for i := range p.Neighbors {
		nb := &p.Neighbors[i]
		enc.BeginSubMsgElem()
		enc.WriteUint(FieldNeighPeerASN, uint64(nb.PeerASN))
		enc.WriteString(FieldNeighRouter, nb.Router)
		enc.WriteUint(FieldNeighLatencyUS, uint64(nb.LatencyUS))
		enc.WriteBool(FieldNeighEstablished, nb.Established)
		enc.EndSubMsgElem()
	}
	enc.EndSubMsgArr()

	enc.WriteUint(FieldPeerSequence, p.Sequence)
	return enc.Finish()
}

// EncodePeerOmniBufDirect encodes PeerUpdate directly into dst []byte in a single forward pass
// (analogous to protowire.Append* operating on a slice in registers, without a two-pass Size() walk).
func EncodePeerOmniBufDirect(dst []byte, p *PeerUpdate) []byte {
	dst = dst[:0]

	// Field 1: ASN (tag = (1<<4)|0 = 16 -> 1B tag 0x21)
	dst = append(dst, byte((FieldPeerASN<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, uint64(p.ASN))

	// Field 2: RouterID (tag = (2<<4)|3 = 35 -> 1B tag)
	dst = append(dst, byte((FieldPeerRouterID<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.RouterID)))
	dst = append(dst, p.RouterID...)

	// Field 3: Active (0-byte boolean in tag)
	if p.Active {
		dst = append(dst, byte((FieldPeerActive<<4|uint16(WireBoolTrue))<<1)|1)
	} else {
		dst = append(dst, byte((FieldPeerActive<<4|uint16(WireBoolFalse))<<1)|1)
	}

	// Field 4: UptimeSec
	dst = append(dst, byte((FieldPeerUptimeSec<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, p.UptimeSec)

	// Field 5: RssiDbm (ZigZag, WireSint)
	dst = append(dst, byte((FieldPeerRssiDbm<<4|uint16(WireSint))<<1)|1)
	dst = AppendVarint(dst, EncodeZigZag(int64(p.RssiDbm)))

	// Field 6: CPULoad (Float64)
	dst = append(dst, byte((FieldPeerCPULoad<<4|uint16(WireFloat64))<<1)|1)
	cpuBits := math.Float64bits(p.CPULoad)
	dst = append(dst,
		byte(cpuBits), byte(cpuBits>>8), byte(cpuBits>>16), byte(cpuBits>>24),
		byte(cpuBits>>32), byte(cpuBits>>40), byte(cpuBits>>48), byte(cpuBits>>56),
	)

	// Field 7: Tags (WireArray of WireString, 1B tag since fieldNum=7, backpatched length)
	dst = append(dst, byte((FieldPeerTags<<4|uint16(WireArray))<<1)|1, 0, 0, WireString)
	tagsLenPos := len(dst) - 3
	dst = AppendVarint(dst, uint64(len(p.Tags)))
	for i := range p.Tags {
		s := p.Tags[i]
		dst = AppendVarint(dst, uint64(len(s)))
		dst = append(dst, s...)
	}
	dst = BackpatchContainerLen(dst, tagsLenPos)

	// Field 8: Prefixes (WireArray of WireUint, 2B tag since fieldNum=8, backpatched length)
	tag8 := ((FieldPeerPrefixes<<4 | uint16(WireArray)) << 2) | 2
	dst = append(dst, byte(tag8), byte(tag8>>8), 0, 0, WireUint)
	prefLenPos := len(dst) - 3
	dst = AppendVarint(dst, uint64(len(p.Prefixes)))
	for i := range p.Prefixes {
		dst = AppendVarint(dst, uint64(p.Prefixes[i]))
	}
	dst = BackpatchContainerLen(dst, prefLenPos)

	// Field 9: Location (WireSubMsg, 1-pass backpatched)
	tag9 := ((FieldPeerLocation<<4 | uint16(WireSubMsg)) << 2) | 2
	dst = append(dst, byte(tag9), byte(tag9>>8), 0, 0)
	locLenPos := len(dst) - 2
	dst = append(dst, byte((FieldLocRegion<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.Location.Region)))
	dst = append(dst, p.Location.Region...)
	dst = append(dst, byte((FieldLocDatacenter<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.Location.Datacenter)))
	dst = append(dst, p.Location.Datacenter...)
	dst = append(dst, byte((FieldLocCity<<4|uint16(WireString))<<1)|1)
	dst = AppendVarint(dst, uint64(len(p.Location.City)))
	dst = append(dst, p.Location.City...)
	dst = append(dst, byte((FieldLocRack<<4|uint16(WireUint))<<1)|1)
	dst = AppendVarint(dst, uint64(p.Location.Rack))
	latBits := math.Float32bits(p.Location.Latitude)
	dst = append(dst, byte((FieldLocLatitude<<4|uint16(WireFloat32))<<1)|1,
		byte(latBits), byte(latBits>>8), byte(latBits>>16), byte(latBits>>24))
	lonBits := math.Float32bits(p.Location.Longitude)
	dst = append(dst, byte((FieldLocLongitude<<4|uint16(WireFloat32))<<1)|1,
		byte(lonBits), byte(lonBits>>8), byte(lonBits>>16), byte(lonBits>>24))
	dst = BackpatchContainerLen(dst, locLenPos)

	// Field 10: Neighbors (WireArray of WireSubMsg, 1-pass backpatched)
	tag10 := ((FieldPeerNeighbors<<4 | uint16(WireArray)) << 2) | 2
	dst = append(dst, byte(tag10), byte(tag10>>8), 0, 0, WireSubMsg)
	arrLenPos := len(dst) - 3
	dst = AppendVarint(dst, uint64(len(p.Neighbors)))
	for i := range p.Neighbors {
		nb := &p.Neighbors[i]
		eLenPos := len(dst)
		dst = append(dst, 0, 0) // Reserve 2B element length
		dst = append(dst, byte((FieldNeighPeerASN<<4|uint16(WireUint))<<1)|1)
		dst = AppendVarint(dst, uint64(nb.PeerASN))
		dst = append(dst, byte((FieldNeighRouter<<4|uint16(WireString))<<1)|1)
		dst = AppendVarint(dst, uint64(len(nb.Router)))
		dst = append(dst, nb.Router...)
		dst = append(dst, byte((FieldNeighLatencyUS<<4|uint16(WireUint))<<1)|1)
		dst = AppendVarint(dst, uint64(nb.LatencyUS))
		if nb.Established {
			dst = append(dst, byte((FieldNeighEstablished<<4|uint16(WireBoolTrue))<<1)|1)
		} else {
			dst = append(dst, byte((FieldNeighEstablished<<4|uint16(WireBoolFalse))<<1)|1)
		}
		dst = BackpatchContainerLen(dst, eLenPos)
	}
	dst = BackpatchContainerLen(dst, arrLenPos)

	// Field 11: Sequence
	tag11 := ((FieldPeerSequence<<4 | uint16(WireUint)) << 2) | 2
	dst = append(dst, byte(tag11), byte(tag11>>8))
	dst = AppendVarint(dst, p.Sequence)
	return dst
}

// DecodePeerOmniBufInto decodes an OmniBuf packet into out.
// If zeroCopyStrings is true, strings reference packet memory via unsafe.String (0 string allocs).
func DecodePeerOmniBufInto(packet []byte, out *PeerUpdate, zeroCopyStrings bool) error {
	n := len(packet)
	pos := 0
	var seenMask uint16
	for pos < n {
		b0 := packet[pos]
		var tag uint16
		if (b0 & 1) != 0 {
			tag = uint16(b0 >> 1)
			pos++
		} else {
			tag = binary.LittleEndian.Uint16(packet[pos:]) >> 2
			pos += 2
		}
		fieldNum := tag >> 4
		wt := uint8(tag & 0x0F)

		switch fieldNum {
		case 0:
			var byteLen uint64
			byteLen, pos = ReadVarintAt(packet, pos)
			pos += int(byteLen)
		case FieldPeerASN:
			var v uint64
			v, pos = ReadVarintAt(packet, pos)
			out.ASN = uint32(v)
		case FieldPeerRouterID:
			var sLen64 uint64
			sLen64, pos = ReadVarintAt(packet, pos)
			sLen := int(sLen64)
			out.RouterID = DecodeStr(packet[pos:pos+sLen], zeroCopyStrings)
			pos += sLen
		case FieldPeerActive:
			out.Active = (wt == WireBoolTrue)
		case FieldPeerUptimeSec:
			out.UptimeSec, pos = ReadVarintAt(packet, pos)
		case FieldPeerRssiDbm:
			var v uint64
			v, pos = ReadVarintAt(packet, pos)
			out.RssiDbm = int32(DecodeZigZag(v))
		case FieldPeerCPULoad:
			out.CPULoad = math.Float64frombits(binary.LittleEndian.Uint64(packet[pos:]))
			pos += 8
		case FieldPeerTags:
			var byteLen64 uint64
			byteLen64, pos = ReadVarintAt(packet, pos)
			end := pos + int(byteLen64)
			var c64 uint64
			c64, p := ReadVarintAt(packet, pos+1)
			alreadySeen := (seenMask & (1 << FieldPeerTags)) != 0
			seenMask |= (1 << FieldPeerTags)
			var start int
			out.Tags, start = growSlice(sliceForDecode(&out.Tags, alreadySeen), int(c64))
			tags := out.Tags[start:]
			for i := range tags {
				var sLen64 uint64
				sLen64, p = ReadVarintAt(packet, p)
				sLen := int(sLen64)
				tags[i] = DecodeStr(packet[p:p+sLen], zeroCopyStrings)
				p += sLen
			}
			pos = end
		case FieldPeerPrefixes:
			var byteLen64 uint64
			byteLen64, pos = ReadVarintAt(packet, pos)
			end := pos + int(byteLen64)
			var c64 uint64
			c64, p := ReadVarintAt(packet, pos+1)
			alreadySeen := (seenMask & (1 << FieldPeerPrefixes)) != 0
			seenMask |= (1 << FieldPeerPrefixes)
			var start int
			out.Prefixes, start = growSlice(sliceForDecode(&out.Prefixes, alreadySeen), int(c64))
			prefs := out.Prefixes[start:]
			for i := range prefs {
				var v uint64
				v, p = ReadVarintAt(packet, p)
				prefs[i] = uint32(v)
			}
			pos = end
		case FieldPeerLocation:
			var subLen64 uint64
			subLen64, pos = ReadVarintAt(packet, pos)
			subLen := int(subLen64)
			decodeLocationOmniBuf(packet[pos:pos+subLen], &out.Location, zeroCopyStrings)
			pos += subLen
		case FieldPeerNeighbors:
			var arrLen64 uint64
			arrLen64, pos = ReadVarintAt(packet, pos)
			arrLen := int(arrLen64)
			arrPayload := packet[pos : pos+arrLen]
			_, elemCount, p, _ := ParseArrayHeader(arrPayload)
			alreadySeen := (seenMask & (1 << FieldPeerNeighbors)) != 0
			seenMask |= (1 << FieldPeerNeighbors)
			var start int
			out.Neighbors, start = growSlice(sliceForDecode(&out.Neighbors, alreadySeen), elemCount)
			neighs := out.Neighbors[start:]
			for i := range elemCount {
				var elemLen64 uint64
				elemLen64, p = ReadVarintAt(arrPayload, p)
				elemLen := int(elemLen64)
				decodeNeighborOmniBuf(arrPayload[p:p+elemLen], &neighs[i], zeroCopyStrings)
				p += elemLen
			}
			pos += arrLen
		case FieldPeerSequence:
			out.Sequence, pos = ReadVarintAt(packet, pos)
		default:
			return fmt.Errorf("omnibuf: unknown field %d", fieldNum)
		}
	}
	return nil
}

func decodeLocationOmniBuf(packet []byte, loc *Location, zeroCopy bool) {
	n := len(packet)
	pos := 0
	for pos < n {
		fieldNum := uint16(packet[pos] >> 5)
		pos++
		switch fieldNum {
		case 0:
			var byteLen uint64
			byteLen, pos = ReadVarintAt(packet, pos)
			pos += int(byteLen)
		case FieldLocRegion:
			var sLen64 uint64
			sLen64, pos = ReadVarintAt(packet, pos)
			sLen := int(sLen64)
			loc.Region = DecodeStr(packet[pos:pos+sLen], zeroCopy)
			pos += sLen
		case FieldLocDatacenter:
			var sLen64 uint64
			sLen64, pos = ReadVarintAt(packet, pos)
			sLen := int(sLen64)
			loc.Datacenter = DecodeStr(packet[pos:pos+sLen], zeroCopy)
			pos += sLen
		case FieldLocCity:
			var sLen64 uint64
			sLen64, pos = ReadVarintAt(packet, pos)
			sLen := int(sLen64)
			loc.City = DecodeStr(packet[pos:pos+sLen], zeroCopy)
			pos += sLen
		case FieldLocRack:
			var v uint64
			v, pos = ReadVarintAt(packet, pos)
			loc.Rack = uint32(v)
		case FieldLocLatitude:
			loc.Latitude = math.Float32frombits(binary.LittleEndian.Uint32(packet[pos:]))
			pos += 4
		case FieldLocLongitude:
			loc.Longitude = math.Float32frombits(binary.LittleEndian.Uint32(packet[pos:]))
			pos += 4
		}
	}
}

func decodeNeighborOmniBuf(packet []byte, nb *Neighbor, zeroCopy bool) {
	n := len(packet)
	pos := 0
	for pos < n {
		tag := packet[pos] >> 1
		pos++
		fieldNum := uint16(tag >> 4)
		switch fieldNum {
		case 0:
			var byteLen uint64
			byteLen, pos = ReadVarintAt(packet, pos)
			pos += int(byteLen)
		case FieldNeighPeerASN:
			var v uint64
			v, pos = ReadVarintAt(packet, pos)
			nb.PeerASN = uint32(v)
		case FieldNeighRouter:
			var sLen64 uint64
			sLen64, pos = ReadVarintAt(packet, pos)
			sLen := int(sLen64)
			nb.Router = DecodeStr(packet[pos:pos+sLen], zeroCopy)
			pos += sLen
		case FieldNeighLatencyUS:
			var v uint64
			v, pos = ReadVarintAt(packet, pos)
			nb.LatencyUS = uint32(v)
		case FieldNeighEstablished:
			nb.Established = ((tag & 0x0F) == WireBoolTrue)
		}
	}
}

// ============================================================================
// 2. Classic Protobuf Wire Encoding (direct protowire)
// ============================================================================

// SizeLocationProto computes the exact LEB128 Protobuf wire size of Location (Pass 1 of 2-pass Protobuf).
func SizeLocationProto(loc *Location) int {
	sz := 0
	sz += 1 + protowire.SizeBytes(len(loc.Region))
	sz += 1 + protowire.SizeBytes(len(loc.Datacenter))
	sz += 1 + protowire.SizeBytes(len(loc.City))
	sz += 1 + protowire.SizeVarint(uint64(loc.Rack))
	sz += 1 + 4 // latitude fixed32
	sz += 1 + 4 // longitude fixed32
	return sz
}

// SizeNeighborProto computes the exact LEB128 Protobuf wire size of Neighbor (Pass 1 of 2-pass Protobuf).
func SizeNeighborProto(nb *Neighbor) int {
	sz := 0
	sz += 1 + protowire.SizeVarint(uint64(nb.PeerASN))
	sz += 1 + protowire.SizeBytes(len(nb.Router))
	sz += 1 + protowire.SizeVarint(uint64(nb.LatencyUS))
	sz += 1 + 1 // established bool
	return sz
}

// EncodePeerProtoWire encodes PeerUpdate using classic Protobuf TLV + LEB128 (2-pass Size + Write).
func EncodePeerProtoWire(dst []byte, p *PeerUpdate) []byte {
	dst = dst[:0]
	// Field 1: asn
	dst = protowire.AppendTag(dst, 1, protowire.VarintType)
	dst = protowire.AppendVarint(dst, uint64(p.ASN))
	// Field 2: router_id
	dst = protowire.AppendTag(dst, 2, protowire.BytesType)
	dst = protowire.AppendString(dst, p.RouterID)
	// Field 3: active
	dst = protowire.AppendTag(dst, 3, protowire.VarintType)
	if p.Active {
		dst = protowire.AppendVarint(dst, 1)
	} else {
		dst = protowire.AppendVarint(dst, 0)
	}
	// Field 4: uptime_sec
	dst = protowire.AppendTag(dst, 4, protowire.VarintType)
	dst = protowire.AppendVarint(dst, p.UptimeSec)
	// Field 5: rssi_dbm (sint32 ZigZag)
	dst = protowire.AppendTag(dst, 5, protowire.VarintType)
	dst = protowire.AppendVarint(dst, protowire.EncodeZigZag(int64(p.RssiDbm)))
	// Field 6: cpu_load (double fixed64)
	dst = protowire.AppendTag(dst, 6, protowire.Fixed64Type)
	dst = protowire.AppendFixed64(dst, math.Float64bits(p.CPULoad))
	// Field 7: repeated string tags
	for i := range p.Tags {
		dst = protowire.AppendTag(dst, 7, protowire.BytesType)
		dst = protowire.AppendString(dst, p.Tags[i])
	}
	// Field 8: packed repeated uint32 prefixes (requires computing packed byte length first!)
	packedLen := 0
	for i := range p.Prefixes {
		packedLen += protowire.SizeVarint(uint64(p.Prefixes[i]))
	}
	dst = protowire.AppendTag(dst, 8, protowire.BytesType)
	dst = protowire.AppendVarint(dst, uint64(packedLen))
	for i := range p.Prefixes {
		dst = protowire.AppendVarint(dst, uint64(p.Prefixes[i]))
	}
	// Field 9: Location sub-message (requires SizeLocationProto pre-pass!)
	locSize := SizeLocationProto(&p.Location)
	dst = protowire.AppendTag(dst, 9, protowire.BytesType)
	dst = protowire.AppendVarint(dst, uint64(locSize))
	dst = protowire.AppendTag(dst, 1, protowire.BytesType)
	dst = protowire.AppendString(dst, p.Location.Region)
	dst = protowire.AppendTag(dst, 2, protowire.BytesType)
	dst = protowire.AppendString(dst, p.Location.Datacenter)
	dst = protowire.AppendTag(dst, 3, protowire.BytesType)
	dst = protowire.AppendString(dst, p.Location.City)
	dst = protowire.AppendTag(dst, 4, protowire.VarintType)
	dst = protowire.AppendVarint(dst, uint64(p.Location.Rack))
	dst = protowire.AppendTag(dst, 5, protowire.Fixed32Type)
	dst = protowire.AppendFixed32(dst, math.Float32bits(p.Location.Latitude))
	dst = protowire.AppendTag(dst, 6, protowire.Fixed32Type)
	dst = protowire.AppendFixed32(dst, math.Float32bits(p.Location.Longitude))
	// Field 10: repeated Neighbor sub-messages (each requires SizeNeighborProto pre-pass!)
	for i := range p.Neighbors {
		nb := &p.Neighbors[i]
		nbSize := SizeNeighborProto(nb)
		dst = protowire.AppendTag(dst, 10, protowire.BytesType)
		dst = protowire.AppendVarint(dst, uint64(nbSize))
		dst = protowire.AppendTag(dst, 1, protowire.VarintType)
		dst = protowire.AppendVarint(dst, uint64(nb.PeerASN))
		dst = protowire.AppendTag(dst, 2, protowire.BytesType)
		dst = protowire.AppendString(dst, nb.Router)
		dst = protowire.AppendTag(dst, 3, protowire.VarintType)
		dst = protowire.AppendVarint(dst, uint64(nb.LatencyUS))
		dst = protowire.AppendTag(dst, 4, protowire.VarintType)
		if nb.Established {
			dst = protowire.AppendVarint(dst, 1)
		} else {
			dst = protowire.AppendVarint(dst, 0)
		}
	}
	// Field 11: sequence
	dst = protowire.AppendTag(dst, 11, protowire.VarintType)
	dst = protowire.AppendVarint(dst, p.Sequence)
	return dst
}

// DecodePeerProtoWireInto decodes a classic Protobuf packet into out using direct protowire.
func DecodePeerProtoWireInto(b []byte, out *PeerUpdate, zeroCopy bool) error {
	out.Tags = out.Tags[:0]
	out.Prefixes = out.Prefixes[:0]
	out.Neighbors = out.Neighbors[:0]
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		switch num {
		case 1:
			v, vn := protowire.ConsumeVarint(b)
			b = b[vn:]
			out.ASN = uint32(v)
		case 2:
			s, sn := protowire.ConsumeBytes(b)
			b = b[sn:]
			out.RouterID = DecodeStr(s, zeroCopy)
		case 3:
			v, vn := protowire.ConsumeVarint(b)
			b = b[vn:]
			out.Active = v != 0
		case 4:
			v, vn := protowire.ConsumeVarint(b)
			b = b[vn:]
			out.UptimeSec = v
		case 5:
			v, vn := protowire.ConsumeVarint(b)
			b = b[vn:]
			out.RssiDbm = int32(protowire.DecodeZigZag(v))
		case 6:
			v, vn := protowire.ConsumeFixed64(b)
			b = b[vn:]
			out.CPULoad = math.Float64frombits(v)
		case 7:
			s, sn := protowire.ConsumeBytes(b)
			b = b[sn:]
			out.Tags = append(out.Tags, DecodeStr(s, zeroCopy))
		case 8:
			packed, pn := protowire.ConsumeBytes(b)
			b = b[pn:]
			for len(packed) > 0 {
				v, vn := protowire.ConsumeVarint(packed)
				packed = packed[vn:]
				out.Prefixes = append(out.Prefixes, uint32(v))
			}
		case 9:
			sub, sn := protowire.ConsumeBytes(b)
			b = b[sn:]
			for len(sub) > 0 {
				fnum, _, fn := protowire.ConsumeTag(sub)
				sub = sub[fn:]
				switch fnum {
				case 1:
					s, vn := protowire.ConsumeBytes(sub)
					sub = sub[vn:]
					out.Location.Region = DecodeStr(s, zeroCopy)
				case 2:
					s, vn := protowire.ConsumeBytes(sub)
					sub = sub[vn:]
					out.Location.Datacenter = DecodeStr(s, zeroCopy)
				case 3:
					s, vn := protowire.ConsumeBytes(sub)
					sub = sub[vn:]
					out.Location.City = DecodeStr(s, zeroCopy)
				case 4:
					v, vn := protowire.ConsumeVarint(sub)
					sub = sub[vn:]
					out.Location.Rack = uint32(v)
				case 5:
					v, vn := protowire.ConsumeFixed32(sub)
					sub = sub[vn:]
					out.Location.Latitude = math.Float32frombits(v)
				case 6:
					v, vn := protowire.ConsumeFixed32(sub)
					sub = sub[vn:]
					out.Location.Longitude = math.Float32frombits(v)
				}
			}
		case 10:
			sub, sn := protowire.ConsumeBytes(b)
			b = b[sn:]
			var nb Neighbor
			for len(sub) > 0 {
				fnum, _, fn := protowire.ConsumeTag(sub)
				sub = sub[fn:]
				switch fnum {
				case 1:
					v, vn := protowire.ConsumeVarint(sub)
					sub = sub[vn:]
					nb.PeerASN = uint32(v)
				case 2:
					s, vn := protowire.ConsumeBytes(sub)
					sub = sub[vn:]
					nb.Router = DecodeStr(s, zeroCopy)
				case 3:
					v, vn := protowire.ConsumeVarint(sub)
					sub = sub[vn:]
					nb.LatencyUS = uint32(v)
				case 4:
					v, vn := protowire.ConsumeVarint(sub)
					sub = sub[vn:]
					nb.Established = v != 0
				}
			}
			out.Neighbors = append(out.Neighbors, nb)
		case 11:
			v, vn := protowire.ConsumeVarint(b)
			b = b[vn:]
			out.Sequence = v
		default:
			vn := protowire.ConsumeFieldValue(num, wtype, b)
			b = b[vn:]
		}
	}
	return nil
}

// LookupProtoWireNestedCity scans a Protobuf packet sequentially to extract peer.location.city
// without allocating structs (the fastest possible way to read a nested field in Protobuf).
func LookupProtoWireNestedCity(b []byte, zeroCopy bool) (string, bool) {
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", false
		}
		b = b[n:]
		if num == 9 { // location
			sub, sn := protowire.ConsumeBytes(b)
			if sn < 0 {
				return "", false
			}
			for len(sub) > 0 {
				fnum, fwtype, fn := protowire.ConsumeTag(sub)
				if fn < 0 {
					return "", false
				}
				sub = sub[fn:]
				if fnum == 3 { // city
					s, vn := protowire.ConsumeBytes(sub)
					if vn < 0 {
						return "", false
					}
					return DecodeStr(s, zeroCopy), true
				}
				vn := protowire.ConsumeFieldValue(fnum, fwtype, sub)
				if vn < 0 {
					return "", false
				}
				sub = sub[vn:]
			}
			return "", false
		}
		vn := protowire.ConsumeFieldValue(num, wtype, b)
		if vn < 0 {
			return "", false
		}
		b = b[vn:]
	}
	return "", false
}

// LookupProtoWireNeighborRouter scans a Protobuf packet sequentially to extract peer.neighbors[targetIdx].router.
func LookupProtoWireNeighborRouter(b []byte, targetIdx int, zeroCopy bool) (string, bool) {
	seen := 0
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", false
		}
		b = b[n:]
		if num == 10 { // neighbors
			sub, sn := protowire.ConsumeBytes(b)
			if sn < 0 {
				return "", false
			}
			if seen == targetIdx {
				for len(sub) > 0 {
					fnum, fwtype, fn := protowire.ConsumeTag(sub)
					if fn < 0 {
						return "", false
					}
					sub = sub[fn:]
					if fnum == 2 { // router
						s, vn := protowire.ConsumeBytes(sub)
						if vn < 0 {
							return "", false
						}
						return DecodeStr(s, zeroCopy), true
					}
					vn := protowire.ConsumeFieldValue(fnum, fwtype, sub)
					if vn < 0 {
						return "", false
					}
					sub = sub[vn:]
				}
				return "", false
			}
			seen++
			b = b[sn:]
			continue
		}
		vn := protowire.ConsumeFieldValue(num, wtype, b)
		if vn < 0 {
			return "", false
		}
		b = b[vn:]
	}
	return "", false
}

// ============================================================================
// 3. FlatBuffers (github.com/google/flatbuffers/go)
// ============================================================================

// EncodePeerFlatBuffers builds a FlatBuffers packet for PeerUpdate using flatbuffers.Builder.
// Notice that FlatBuffers requires inside-out (bottom-up) serialization: all strings, nested
// tables, and offset vectors must be serialized BEFORE their parent table can be started.
func EncodePeerFlatBuffers(b *flatbuffers.Builder, p *PeerUpdate) []byte {
	b.Reset()

	// 1. Serialize Location strings & Location table first
	regOff := b.CreateString(p.Location.Region)
	dcOff := b.CreateString(p.Location.Datacenter)
	cityOff := b.CreateString(p.Location.City)

	b.StartObject(6)
	b.PrependUOffsetTSlot(0, regOff, 0)
	b.PrependUOffsetTSlot(1, dcOff, 0)
	b.PrependUOffsetTSlot(2, cityOff, 0)
	b.PrependUint32Slot(3, p.Location.Rack, 0)
	b.PrependFloat32Slot(4, p.Location.Latitude, 0)
	b.PrependFloat32Slot(5, p.Location.Longitude, 0)
	locOff := b.EndObject()

	// 2. Serialize each Neighbor string & Neighbor table, then Neighbors vector
	var neighOffs [16]flatbuffers.UOffsetT
	nCount := len(p.Neighbors)
	for i := nCount - 1; i >= 0; i-- {
		nb := &p.Neighbors[i]
		rtrOff := b.CreateString(nb.Router)
		b.StartObject(4)
		b.PrependUint32Slot(0, nb.PeerASN, 0)
		b.PrependUOffsetTSlot(1, rtrOff, 0)
		b.PrependUint32Slot(2, nb.LatencyUS, 0)
		b.PrependBoolSlot(3, nb.Established, false)
		neighOffs[i] = b.EndObject()
	}
	b.StartVector(4, nCount, 4)
	for i := nCount - 1; i >= 0; i-- {
		b.PrependUOffsetT(neighOffs[i])
	}
	neighborsVecOff := b.EndVector(nCount)

	// 3. Serialize Tags strings and vector
	var tagOffs [16]flatbuffers.UOffsetT
	tCount := len(p.Tags)
	for i := tCount - 1; i >= 0; i-- {
		tagOffs[i] = b.CreateString(p.Tags[i])
	}
	b.StartVector(4, tCount, 4)
	for i := tCount - 1; i >= 0; i-- {
		b.PrependUOffsetT(tagOffs[i])
	}
	tagsVecOff := b.EndVector(tCount)

	// 4. Serialize Prefixes uint32 vector
	pCount := len(p.Prefixes)
	b.StartVector(4, pCount, 4)
	for i := pCount - 1; i >= 0; i-- {
		b.PrependUint32(p.Prefixes[i])
	}
	prefixesVecOff := b.EndVector(pCount)

	// 5. Serialize RouterID string and root PeerUpdate table
	routerIDOff := b.CreateString(p.RouterID)

	b.StartObject(11)
	b.PrependUint32Slot(0, p.ASN, 0)
	b.PrependUOffsetTSlot(1, routerIDOff, 0)
	b.PrependBoolSlot(2, p.Active, false)
	b.PrependUint64Slot(3, p.UptimeSec, 0)
	b.PrependInt32Slot(4, p.RssiDbm, 0)
	b.PrependFloat64Slot(5, p.CPULoad, 0)
	b.PrependUOffsetTSlot(6, tagsVecOff, 0)
	b.PrependUOffsetTSlot(7, prefixesVecOff, 0)
	b.PrependUOffsetTSlot(8, locOff, 0)
	b.PrependUOffsetTSlot(9, neighborsVecOff, 0)
	b.PrependUint64Slot(10, p.Sequence, 0)
	root := b.EndObject()

	b.Finish(root)
	return b.FinishedBytes()
}

// errFlatBuffersInvalidUTF8 reports a FlatBuffers string that violates the
// protobuf string contract used by the equivalent PeerUpdate schema.
var errFlatBuffersInvalidUTF8 = errors.New("FlatBuffers string contains invalid UTF-8")

func decodeFlatBuffersString(b []byte, zeroCopy bool) (string, bool) {
	if !ValidUTF8(b) {
		return "", false
	}
	return DecodeStr(b, zeroCopy), true
}

// DecodePeerFlatBuffersInto decodes a FlatBuffers PeerUpdate packet into out.
// It checks every present string field, including vector elements.
func DecodePeerFlatBuffersInto(buf []byte, out *PeerUpdate, zeroCopy bool) error {
	rootOff := flatbuffers.GetUOffsetT(buf)
	tab := flatbuffers.Table{Bytes: buf, Pos: rootOff}

	if o := flatbuffers.UOffsetT(tab.Offset(4)); o != 0 {
		out.ASN = tab.GetUint32(o + tab.Pos)
	}
	if o := flatbuffers.UOffsetT(tab.Offset(6)); o != 0 {
		if value, ok := decodeFlatBuffersString(tab.ByteVector(o+tab.Pos), zeroCopy); !ok {
			return errFlatBuffersInvalidUTF8
		} else {
			out.RouterID = value
		}
	}
	if o := flatbuffers.UOffsetT(tab.Offset(8)); o != 0 {
		out.Active = tab.GetBool(o + tab.Pos)
	}
	if o := flatbuffers.UOffsetT(tab.Offset(10)); o != 0 {
		out.UptimeSec = tab.GetUint64(o + tab.Pos)
	}
	if o := flatbuffers.UOffsetT(tab.Offset(12)); o != 0 {
		out.RssiDbm = tab.GetInt32(o + tab.Pos)
	}
	if o := flatbuffers.UOffsetT(tab.Offset(14)); o != 0 {
		out.CPULoad = tab.GetFloat64(o + tab.Pos)
	}
	if o := flatbuffers.UOffsetT(tab.Offset(16)); o != 0 {
		vecLen := tab.VectorLen(o)
		if cap(out.Tags) < vecLen {
			out.Tags = make([]string, vecLen)
		} else {
			out.Tags = out.Tags[:vecLen]
		}
		vecStart := tab.Vector(o)
		for i := range vecLen {
			elemPos := vecStart + flatbuffers.UOffsetT(i*4)
			if value, ok := decodeFlatBuffersString(tab.ByteVector(elemPos), zeroCopy); !ok {
				return errFlatBuffersInvalidUTF8
			} else {
				out.Tags[i] = value
			}
		}
	}
	if o := flatbuffers.UOffsetT(tab.Offset(18)); o != 0 {
		vecLen := tab.VectorLen(o)
		if cap(out.Prefixes) < vecLen {
			out.Prefixes = make([]uint32, vecLen)
		} else {
			out.Prefixes = out.Prefixes[:vecLen]
		}
		vecStart := tab.Vector(o)
		for i := range vecLen {
			out.Prefixes[i] = tab.GetUint32(vecStart + flatbuffers.UOffsetT(i*4))
		}
	}
	if o := flatbuffers.UOffsetT(tab.Offset(20)); o != 0 {
		locPos := tab.Indirect(o + tab.Pos)
		locTab := flatbuffers.Table{Bytes: buf, Pos: locPos}
		if lo := flatbuffers.UOffsetT(locTab.Offset(4)); lo != 0 {
			if value, ok := decodeFlatBuffersString(locTab.ByteVector(lo+locTab.Pos), zeroCopy); !ok {
				return errFlatBuffersInvalidUTF8
			} else {
				out.Location.Region = value
			}
		}
		if lo := flatbuffers.UOffsetT(locTab.Offset(6)); lo != 0 {
			if value, ok := decodeFlatBuffersString(locTab.ByteVector(lo+locTab.Pos), zeroCopy); !ok {
				return errFlatBuffersInvalidUTF8
			} else {
				out.Location.Datacenter = value
			}
		}
		if lo := flatbuffers.UOffsetT(locTab.Offset(8)); lo != 0 {
			if value, ok := decodeFlatBuffersString(locTab.ByteVector(lo+locTab.Pos), zeroCopy); !ok {
				return errFlatBuffersInvalidUTF8
			} else {
				out.Location.City = value
			}
		}
		if lo := flatbuffers.UOffsetT(locTab.Offset(10)); lo != 0 {
			out.Location.Rack = locTab.GetUint32(lo + locTab.Pos)
		}
		if lo := flatbuffers.UOffsetT(locTab.Offset(12)); lo != 0 {
			out.Location.Latitude = locTab.GetFloat32(lo + locTab.Pos)
		}
		if lo := flatbuffers.UOffsetT(locTab.Offset(14)); lo != 0 {
			out.Location.Longitude = locTab.GetFloat32(lo + locTab.Pos)
		}
	}
	if o := flatbuffers.UOffsetT(tab.Offset(22)); o != 0 {
		vecLen := tab.VectorLen(o)
		if cap(out.Neighbors) < vecLen {
			out.Neighbors = make([]Neighbor, vecLen)
		} else {
			out.Neighbors = out.Neighbors[:vecLen]
		}
		vecStart := tab.Vector(o)
		for i := range vecLen {
			nbPos := tab.Indirect(vecStart + flatbuffers.UOffsetT(i*4))
			nbTab := flatbuffers.Table{Bytes: buf, Pos: nbPos}
			if no := flatbuffers.UOffsetT(nbTab.Offset(4)); no != 0 {
				out.Neighbors[i].PeerASN = nbTab.GetUint32(no + nbTab.Pos)
			}
			if no := flatbuffers.UOffsetT(nbTab.Offset(6)); no != 0 {
				if value, ok := decodeFlatBuffersString(nbTab.ByteVector(no+nbTab.Pos), zeroCopy); !ok {
					return errFlatBuffersInvalidUTF8
				} else {
					out.Neighbors[i].Router = value
				}
			}
			if no := flatbuffers.UOffsetT(nbTab.Offset(8)); no != 0 {
				out.Neighbors[i].LatencyUS = nbTab.GetUint32(no + nbTab.Pos)
			}
			if no := flatbuffers.UOffsetT(nbTab.Offset(10)); no != 0 {
				out.Neighbors[i].Established = nbTab.GetBool(no + nbTab.Pos)
			}
		}
	}
	if o := flatbuffers.UOffsetT(tab.Offset(24)); o != 0 {
		out.Sequence = tab.GetUint64(o + tab.Pos)
	}
	return nil
}

// LookupFlatBuffersNestedCity reads peer.location.city via FlatBuffers vtable indirection.
func LookupFlatBuffersNestedCity(buf []byte, zeroCopy bool) (string, bool) {
	rootOff := flatbuffers.GetUOffsetT(buf)
	tab := flatbuffers.Table{Bytes: buf, Pos: rootOff}
	o := flatbuffers.UOffsetT(tab.Offset(20)) // slot 8 (location): 4 + 8*2 = 20
	if o == 0 {
		return "", false
	}
	locPos := tab.Indirect(o + tab.Pos)
	locTab := flatbuffers.Table{Bytes: buf, Pos: locPos}
	lo := flatbuffers.UOffsetT(locTab.Offset(8)) // slot 2 (city): 4 + 2*2 = 8
	if lo == 0 {
		return "", false
	}
	return decodeFlatBuffersString(locTab.ByteVector(lo+locTab.Pos), zeroCopy)
}

// LookupFlatBuffersNeighborRouter reads peer.neighbors[idx].router via FlatBuffers vtable + vector indirection.
func LookupFlatBuffersNeighborRouter(buf []byte, idx int, zeroCopy bool) (string, bool) {
	rootOff := flatbuffers.GetUOffsetT(buf)
	tab := flatbuffers.Table{Bytes: buf, Pos: rootOff}
	o := flatbuffers.UOffsetT(tab.Offset(22)) // slot 9 (neighbors): 4 + 9*2 = 22
	if o == 0 || idx >= tab.VectorLen(o) {
		return "", false
	}
	vecStart := tab.Vector(o)
	nbPos := tab.Indirect(vecStart + flatbuffers.UOffsetT(idx*4))
	nbTab := flatbuffers.Table{Bytes: buf, Pos: nbPos}
	no := flatbuffers.UOffsetT(nbTab.Offset(6)) // slot 1 (router): 4 + 1*2 = 6
	if no == 0 {
		return "", false
	}
	return decodeFlatBuffersString(nbTab.ByteVector(no+nbTab.Pos), zeroCopy)
}

// ============================================================================
// 4. Cap'n Proto (capnproto.org/go/capnp/v3)
// ============================================================================
//
// Cap'n Proto struct layout for PeerUpdate:
//   Data section (5 words = 40 bytes):
//     offset 0 (uint32):  ASN
//     offset 4 (bool):    Active (bit 32)
//     offset 8 (uint64):  UptimeSec
//     offset 16 (int32):  RssiDbm
//     offset 24 (float64): CPULoad
//     offset 32 (uint64): Sequence
//   Pointer section (5 pointers = 40 bytes):
//     ptr 0: RouterID (Text)
//     ptr 1: Tags (TextList)
//     ptr 2: Prefixes (UInt32List)
//     ptr 3: Location (Struct: 2 data words [Rack, Lat, Lon], 3 pointers [Region, DC, City])
//     ptr 4: Neighbors (StructList: 2 data words [PeerASN, LatencyUS, Established], 1 pointer [Router])

var (
	capnpPeerSize = capnp.ObjectSize{DataSize: 40, PointerCount: 5}
	capnpLocSize  = capnp.ObjectSize{DataSize: 16, PointerCount: 3}
	capnpNeighSz  = capnp.ObjectSize{DataSize: 16, PointerCount: 1}
)

// EncodePeerCapnp encodes PeerUpdate using capnproto.org/go/capnp/v3.
func EncodePeerCapnp(scratch []byte, p *PeerUpdate) ([]byte, error) {
	msg, seg := capnp.NewSingleSegmentMessage(scratch[:0])
	root, err := capnp.NewRootStruct(seg, capnpPeerSize)
	if err != nil {
		return nil, err
	}

	root.SetUint32(0, p.ASN)
	root.SetBit(32, p.Active)
	root.SetUint64(8, p.UptimeSec)
	root.SetUint32(16, uint32(p.RssiDbm))
	root.SetUint64(24, math.Float64bits(p.CPULoad))
	root.SetUint64(32, p.Sequence)

	if err := root.SetText(0, p.RouterID); err != nil {
		return nil, err
	}

	tagsList, err := capnp.NewTextList(seg, int32(len(p.Tags)))
	if err != nil {
		return nil, err
	}
	for i, t := range p.Tags {
		if err := tagsList.Set(i, t); err != nil {
			return nil, err
		}
	}
	if err := root.SetPtr(1, tagsList.ToPtr()); err != nil {
		return nil, err
	}

	prefList, err := capnp.NewUInt32List(seg, int32(len(p.Prefixes)))
	if err != nil {
		return nil, err
	}
	for i, pr := range p.Prefixes {
		prefList.Set(i, pr)
	}
	if err := root.SetPtr(2, prefList.ToPtr()); err != nil {
		return nil, err
	}

	locSt, err := capnp.NewStruct(seg, capnpLocSize)
	if err != nil {
		return nil, err
	}
	locSt.SetUint32(0, p.Location.Rack)
	locSt.SetUint32(4, math.Float32bits(p.Location.Latitude))
	locSt.SetUint32(8, math.Float32bits(p.Location.Longitude))
	_ = locSt.SetText(0, p.Location.Region)
	_ = locSt.SetText(1, p.Location.Datacenter)
	_ = locSt.SetText(2, p.Location.City)
	if err := root.SetPtr(3, locSt.ToPtr()); err != nil {
		return nil, err
	}

	neighList, err := capnp.NewCompositeList(seg, capnpNeighSz, int32(len(p.Neighbors)))
	if err != nil {
		return nil, err
	}
	for i := range p.Neighbors {
		nb := &p.Neighbors[i]
		elem := neighList.Struct(i)
		elem.SetUint32(0, nb.PeerASN)
		elem.SetUint32(4, nb.LatencyUS)
		elem.SetBit(64, nb.Established)
		_ = elem.SetText(0, nb.Router)
	}
	if err := root.SetPtr(4, neighList.ToPtr()); err != nil {
		return nil, err
	}

	return msg.Marshal()
}

// DecodePeerCapnpInto decodes a Cap'n Proto packet into out.
func DecodePeerCapnpInto(data []byte, out *PeerUpdate, zeroCopy bool) error {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return err
	}
	root, err := msg.Root()
	if err != nil {
		return err
	}
	st := root.Struct()
	out.ASN = st.Uint32(0)
	out.Active = st.Bit(32)
	out.UptimeSec = st.Uint64(8)
	out.RssiDbm = int32(st.Uint32(16))
	out.CPULoad = math.Float64frombits(st.Uint64(24))
	out.Sequence = st.Uint64(32)

	rtrBytes, _ := st.Ptr(0)
	out.RouterID = decodeCapnpText(rtrBytes.TextBytes(), zeroCopy)

	tagsPtr, _ := st.Ptr(1)
	tagsList := capnp.TextList(tagsPtr.List())
	tLen := tagsList.Len()
	if cap(out.Tags) < tLen {
		out.Tags = make([]string, tLen)
	} else {
		out.Tags = out.Tags[:tLen]
	}
	for i := range tLen {
		b, _ := tagsList.BytesAt(i)
		out.Tags[i] = DecodeStr(b, zeroCopy)
	}

	prefPtr, _ := st.Ptr(2)
	prefList := capnp.UInt32List(prefPtr.List())
	pLen := prefList.Len()
	if cap(out.Prefixes) < pLen {
		out.Prefixes = make([]uint32, pLen)
	} else {
		out.Prefixes = out.Prefixes[:pLen]
	}
	for i := range pLen {
		out.Prefixes[i] = prefList.At(i)
	}

	locPtr, _ := st.Ptr(3)
	locSt := locPtr.Struct()
	out.Location.Rack = locSt.Uint32(0)
	out.Location.Latitude = math.Float32frombits(locSt.Uint32(4))
	out.Location.Longitude = math.Float32frombits(locSt.Uint32(8))
	p0, _ := locSt.Ptr(0)
	p1, _ := locSt.Ptr(1)
	p2, _ := locSt.Ptr(2)
	out.Location.Region = decodeCapnpText(p0.TextBytes(), zeroCopy)
	out.Location.Datacenter = decodeCapnpText(p1.TextBytes(), zeroCopy)
	out.Location.City = decodeCapnpText(p2.TextBytes(), zeroCopy)

	neighPtr, _ := st.Ptr(4)
	neighList := neighPtr.List()
	nLen := neighList.Len()
	if cap(out.Neighbors) < nLen {
		out.Neighbors = make([]Neighbor, nLen)
	} else {
		out.Neighbors = out.Neighbors[:nLen]
	}
	for i := range nLen {
		elem := neighList.Struct(i)
		out.Neighbors[i].PeerASN = elem.Uint32(0)
		out.Neighbors[i].LatencyUS = elem.Uint32(4)
		out.Neighbors[i].Established = elem.Bit(64)
		rp, _ := elem.Ptr(0)
		out.Neighbors[i].Router = decodeCapnpText(rp.TextBytes(), zeroCopy)
	}
	return nil
}

func decodeCapnpText(b []byte, zeroCopy bool) string {
	return DecodeStr(b, zeroCopy)
}

// LookupCapnpNestedCity reads peer.location.city from a Cap'n Proto message.
func LookupCapnpNestedCity(data []byte, zeroCopy bool) (string, bool) {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return "", false
	}
	root, err := msg.Root()
	if err != nil {
		return "", false
	}
	locPtr, err := root.Struct().Ptr(3)
	if err != nil {
		return "", false
	}
	cityPtr, err := locPtr.Struct().Ptr(2)
	if err != nil {
		return "", false
	}
	return decodeCapnpText(cityPtr.TextBytes(), zeroCopy), true
}

// LookupCapnpNeighborRouter reads peer.neighbors[idx].router from a Cap'n Proto message.
func LookupCapnpNeighborRouter(data []byte, idx int, zeroCopy bool) (string, bool) {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return "", false
	}
	root, err := msg.Root()
	if err != nil {
		return "", false
	}
	neighPtr, err := root.Struct().Ptr(4)
	if err != nil {
		return "", false
	}
	list := neighPtr.List()
	if idx >= list.Len() {
		return "", false
	}
	elem := list.Struct(idx)
	rtrPtr, err := elem.Ptr(0)
	if err != nil {
		return "", false
	}
	return decodeCapnpText(rtrPtr.TextBytes(), zeroCopy), true
}

// Cap'n Proto In-Place Zero-Allocation Pointer Traversal (bypassing capnp.Unmarshal allocation)
//
// In Cap'n Proto's 64-bit word format:
//   - Framed single-segment buffer starts with 8B segment table header, so segment 0 begins at data[8:].
//   - Struct pointer word: [offset (30b signed, bits 2..31)][dataWords (16b, bits 32..47)][ptrWords (16b, bits 48..63)]
//   - List pointer word:   [offset (30b signed, bits 2..31)][elemSizeCode (3b, bits 32..34)][count/words (29b, bits 35..63)]
//     (For Text/Data list, elemSizeCode=2 (1B elements) and count includes trailing NUL byte.
//      For Composite struct list, elemSizeCode=7 and word 0 of the list payload is the tag word.)

// LookupCapnpInPlaceNestedCity traverses Cap'n Proto's 64-bit pointers directly in-place with 0 allocations.
func LookupCapnpInPlaceNestedCity(data []byte, zeroCopy bool) (string, bool) {
	seg := data[8:] // skip 8-byte single-segment framing header
	// Word 0 is the root struct pointer
	rootPtr := binary.LittleEndian.Uint64(seg[0:8])
	rootOffWords := 1 + int(int32(uint32(rootPtr))>>2)
	rootDataWords := int(uint16(rootPtr >> 32))

	// Pointer 3 in root PeerUpdate is Location
	locPtrPos := (rootOffWords + rootDataWords + 3) * 8
	locPtr := binary.LittleEndian.Uint64(seg[locPtrPos : locPtrPos+8])
	locOffWords := (locPtrPos >> 3) + 1 + int(int32(uint32(locPtr))>>2)
	locDataWords := int(uint16(locPtr >> 32))

	// Pointer 2 in Location is City (Text list pointer, NUL-terminated)
	cityPtrPos := (locOffWords + locDataWords + 2) * 8
	cityPtr := binary.LittleEndian.Uint64(seg[cityPtrPos : cityPtrPos+8])
	cityOffBytes := ((cityPtrPos >> 3) + 1 + int(int32(uint32(cityPtr))>>2)) * 8
	cityLen := int(cityPtr>>35) - 1 // exclude trailing NUL
	if cityLen < 0 {
		return "", false
	}
	return DecodeStr(seg[cityOffBytes:cityOffBytes+cityLen], zeroCopy), true
}

// LookupCapnpInPlaceNeighborRouter traverses Cap'n Proto's composite list + struct pointer in-place with 0 allocations.
func LookupCapnpInPlaceNeighborRouter(data []byte, idx int, zeroCopy bool) (string, bool) {
	seg := data[8:]
	rootPtr := binary.LittleEndian.Uint64(seg[0:8])
	rootOffWords := 1 + int(int32(uint32(rootPtr))>>2)
	rootDataWords := int(uint16(rootPtr >> 32))

	// Pointer 4 in root PeerUpdate is Neighbors (Composite List)
	neighPtrPos := (rootOffWords + rootDataWords + 4) * 8
	neighPtr := binary.LittleEndian.Uint64(seg[neighPtrPos : neighPtrPos+8])
	tagWordIdx := (neighPtrPos >> 3) + 1 + int(int32(uint32(neighPtr))>>2)

	tagWord := binary.LittleEndian.Uint64(seg[tagWordIdx*8 : tagWordIdx*8+8])
	elemCount := int(int32(uint32(tagWord)) >> 2)
	if idx >= elemCount {
		return "", false
	}
	elemDataWords := int(uint16(tagWord >> 32))
	elemPtrWords := int(uint16(tagWord >> 48))
	elemStrideWords := elemDataWords + elemPtrWords

	// Element `idx` starts at tagWordIdx + 1 + idx*elemStrideWords
	elemStartWords := tagWordIdx + 1 + idx*elemStrideWords
	rtrPtrPos := (elemStartWords + elemDataWords + 0) * 8 // Pointer 0 in Neighbor is Router
	rtrPtr := binary.LittleEndian.Uint64(seg[rtrPtrPos : rtrPtrPos+8])
	rtrOffBytes := ((rtrPtrPos >> 3) + 1 + int(int32(uint32(rtrPtr))>>2)) * 8
	rtrLen := int(rtrPtr>>35) - 1
	if rtrLen < 0 {
		return "", false
	}
	return DecodeStr(seg[rtrOffBytes:rtrOffBytes+rtrLen], zeroCopy), true
}

// ============================================================================
// 5. MessagePack (github.com/vmihailenco/msgpack/v5) & JSON (encoding/json)
// ============================================================================

// MarshalMsgpack encodes PeerUpdate using MessagePack.
func MarshalMsgpack(p *PeerUpdate) ([]byte, error) {
	return msgpack.Marshal(p)
}

// UnmarshalMsgpack decodes a MessagePack packet into out.
func UnmarshalMsgpack(b []byte, out *PeerUpdate) error {
	return msgpack.Unmarshal(b, out)
}

// LookupMsgpackNestedCity uses msgpack.Decoder.Query or field skipping to read location.city.
func LookupMsgpackNestedCity(b []byte) (string, bool) {
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.Reset(bytes.NewReader(b))

	n, err := dec.DecodeMapLen()
	if err != nil {
		return "", false
	}
	for range n {
		k, err := dec.DecodeString()
		if err != nil {
			return "", false
		}
		if k == "location" {
			ln, err := dec.DecodeMapLen()
			if err != nil {
				return "", false
			}
			for range ln {
				lk, err := dec.DecodeString()
				if err != nil {
					return "", false
				}
				if lk == "city" {
					val, err := dec.DecodeString()
					return val, err == nil
				}
				if err := dec.Skip(); err != nil {
					return "", false
				}
			}
			return "", false
		}
		if err := dec.Skip(); err != nil {
			return "", false
		}
	}
	return "", false
}

// MarshalJSON encodes PeerUpdate using standard encoding/json.
func MarshalJSON(p *PeerUpdate) ([]byte, error) {
	return json.Marshal(p)
}

// UnmarshalJSON decodes a JSON packet into out.
func UnmarshalJSON(b []byte, out *PeerUpdate) error {
	return json.Unmarshal(b, out)
}

// ============================================================================
// 6. Apache Avro (github.com/hamba/avro/v2) & Apache Parquet (parquet-go)
// ============================================================================

// Avro has no unsigned integer types (uint32/uint64) in its specification, only signed 32-bit int and 64-bit long.
// These struct types have the exact same memory layout as Location, Neighbor, and PeerUpdate so we can cast with zero copy.
type avroLocation struct {
	Region     string  `avro:"region"`
	Datacenter string  `avro:"datacenter"`
	City       string  `avro:"city"`
	Rack       int32   `avro:"rack"`
	Latitude   float32 `avro:"latitude"`
	Longitude  float32 `avro:"longitude"`
}

type avroNeighbor struct {
	PeerASN     int32  `avro:"peer_asn"`
	Router      string `avro:"router"`
	LatencyUS   int32  `avro:"latency_us"`
	Established bool   `avro:"established"`
}

type avroPeerUpdate struct {
	ASN       int32          `avro:"asn"`
	RouterID  string         `avro:"router_id"`
	Active    bool           `avro:"active"`
	UptimeSec int64          `avro:"uptime_sec"`
	RssiDbm   int32          `avro:"rssi_dbm"`
	CPULoad   float64        `avro:"cpu_load"`
	Tags      []string       `avro:"tags"`
	Prefixes  []int32        `avro:"prefixes"`
	Location  avroLocation   `avro:"location"`
	Neighbors []avroNeighbor `avro:"neighbors"`
	Sequence  int64          `avro:"sequence"`
}

var peerAvroSchema = avro.MustParse(`{
  "type": "record",
  "name": "PeerUpdate",
  "namespace": "omnibuf.bench.v1",
  "fields": [
    {"name": "asn", "type": "int"},
    {"name": "router_id", "type": "string"},
    {"name": "active", "type": "boolean"},
    {"name": "uptime_sec", "type": "long"},
    {"name": "rssi_dbm", "type": "int"},
    {"name": "cpu_load", "type": "double"},
    {"name": "tags", "type": {"type": "array", "items": "string"}},
    {"name": "prefixes", "type": {"type": "array", "items": "int"}},
    {"name": "location", "type": {
      "type": "record",
      "name": "Location",
      "fields": [
        {"name": "region", "type": "string"},
        {"name": "datacenter", "type": "string"},
        {"name": "city", "type": "string"},
        {"name": "rack", "type": "int"},
        {"name": "latitude", "type": "float"},
        {"name": "longitude", "type": "float"}
      ]
    }},
    {"name": "neighbors", "type": {"type": "array", "items": {
      "type": "record",
      "name": "Neighbor",
      "fields": [
        {"name": "peer_asn", "type": "int"},
        {"name": "router", "type": "string"},
        {"name": "latency_us", "type": "int"},
        {"name": "established", "type": "boolean"}
      ]
    }}},
    {"name": "sequence", "type": "long"}
  ]
}`)

// MarshalAvro encodes PeerUpdate using raw schema-dependent Avro binary.
func MarshalAvro(p *PeerUpdate) ([]byte, error) {
	return avro.Marshal(peerAvroSchema, (*avroPeerUpdate)(unsafe.Pointer(p)))
}

// UnmarshalAvro decodes raw schema-dependent Avro binary into out.
func UnmarshalAvro(b []byte, out *PeerUpdate) error {
	return avro.Unmarshal(peerAvroSchema, b, (*avroPeerUpdate)(unsafe.Pointer(out)))
}

// MarshalAvroOCF encodes PeerUpdate into a self-describing Avro Object Container File (with embedded JSON schema header).
func MarshalAvroOCF(p *PeerUpdate) ([]byte, error) {
	var buf bytes.Buffer
	enc, err := ocf.NewEncoderWithSchema(peerAvroSchema, &buf)
	if err != nil {
		return nil, err
	}
	if err := enc.Encode((*avroPeerUpdate)(unsafe.Pointer(p))); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LookupAvroNestedCity reads location.city from an Avro binary payload using avro.Reader to skip preceding fields.
func LookupAvroNestedCity(b []byte) (string, bool) {
	r := avro.NewReader(bytes.NewReader(b), len(b))
	r.SkipInt()    // asn
	r.SkipString() // router_id
	r.SkipBool()   // active
	r.SkipLong()   // uptime_sec
	r.SkipInt()    // rssi_dbm
	r.SkipDouble() // cpu_load
	r.ReadArrayCB(func(r *avro.Reader) bool {
		r.SkipString()
		return true
	})
	r.ReadArrayCB(func(r *avro.Reader) bool {
		r.SkipInt()
		return true
	})
	r.SkipString() // location.region
	r.SkipString() // location.datacenter
	city := r.ReadString()
	if r.Error != nil {
		return "", false
	}
	return city, true
}
