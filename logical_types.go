package omnibuf

import "reflect"

// fixedWireType distinguishes signed integers, unsigned integers and floating
// point values everywhere: fields, array elements, and map entries.
func fixedWireType(t reflect.Type, wt uint8) uint8 {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if wt != wireFixed32 && wt != wireFixed64 {
		return wt
	}
	switch t.Kind() {
	case reflect.Float32:
		return wireFloat32
	case reflect.Float64:
		return wireFloat64
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if wt == wireFixed32 {
			return wireSfixed32
		}
		return wireSfixed64
	}
	return wt
}
func finalizeWireType(fc *fieldCodec, t reflect.Type, opt string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if fc.wireType == wireArray {
		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		switch fc.kind {
		case kindStringSlice:
			fc.elemWireType = wireString
			if opt == "bytes" {
				fc.elemWireType = wireBytes
			}
		case kindBytesSlice:
			fc.elemWireType = wireBytes
			if opt == "string" {
				fc.elemWireType = wireString
			}
		case kindBoolSlice:
			fc.elemWireType = wireBoolFalse
		case kindStructValSlice, kindStructPtrSlice:
			fc.elemWireType = wireSubMsg
		case kindUint8Slice, kindUint16Slice, kindUint32Slice, kindUint64Slice:
			fc.elemWireType = wireUint
		case kindInt8Slice, kindInt16Slice, kindInt32Slice, kindInt64Slice, kindUint8SintSlice, kindUint16SintSlice:
			fc.elemWireType = wireSint
		default:
			wt := uint8(wireFixed64)
			switch fc.kind {
			case kindFloat32Slice, kindUint8Fixed32Slice, kindUint16Fixed32Slice, kindUint64Fixed32Slice, kindInt8Fixed32Slice, kindInt16Fixed32Slice, kindInt64Fixed32Slice:
				wt = wireFixed32
			}
			fc.elemWireType = fixedWireType(elem, wt)
		}
	} else {
		fc.wireType = fixedWireType(t, fc.wireType)
	}
	if t.Kind() == reflect.Map {
		fc.mapKeyWT = fixedWireType(t.Key(), fc.mapKeyWT)
		fc.mapValWT = fixedWireType(t.Elem(), fc.mapValWT)
		if fc.mapKeyWT == wireBoolTrue {
			fc.mapKeyWT = wireBoolFalse
		}
		if fc.mapValWT == wireBoolTrue {
			fc.mapValWT = wireBoolFalse
		}
	}
}
