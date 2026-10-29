package omnibuf

import (
	"slices"

	"github.com/sudorandom/omnibuf/wire"
)

func parseMapHeader(body []byte) (keyWT, valWT uint8, count, pos int, ok bool) {
	return wire.ParseMapHeader(body)
}

func encodeMapStringUint(dst []byte, m map[string]uint64, deterministic bool) []byte {
	lenPos := len(dst)
	dst = append(dst, 0, 0, wireString, wireUint)
	dst = appendVarint(dst, uint64(len(m)))
	if deterministic && len(m) > 1 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		if len(keys) <= 16 {
			for i := 1; i < len(keys); i++ {
				for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
					keys[j], keys[j-1] = keys[j-1], keys[j]
				}
			}
		} else {
			slices.Sort(keys)
		}
		for _, k := range keys {
			dst = appendVarint(dst, uint64(len(k)))
			dst = append(dst, k...)
			dst = appendVarint(dst, m[k])
		}
	} else {
		for k, v := range m {
			dst = appendVarint(dst, uint64(len(k)))
			dst = append(dst, k...)
			dst = appendVarint(dst, v)
		}
	}
	return backpatchContainerLen(dst, lenPos)
}

func decodeMapStringUintBody(dst map[string]uint64, body []byte, zeroCopy bool) (map[string]uint64, bool) {
	keyWT, valWT, count, p, ok := parseMapHeader(body)
	if !ok || keyWT != wireString || valWT != wireUint {
		return nil, false
	}
	if dst == nil {
		dst = make(map[string]uint64, count)
	}
	for range count {
		kLen, kn := decodeVarint(body[p:])
		if kn == 0 || kLen > uint64(len(body)-(p+kn)) {
			return nil, false
		}
		p += kn
		if !wire.ValidUTF8(body[p : p+int(kLen)]) {
			return nil, false
		}
		k := decodeStr(body[p:p+int(kLen)], zeroCopy)
		p += int(kLen)
		v, vn := decodeVarint(body[p:])
		if vn == 0 {
			return nil, false
		}
		p += vn
		dst[k] = v
	}
	if p != len(body) {
		return nil, false
	}
	return dst, true
}
