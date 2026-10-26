package omnibuf

import (
	"reflect"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var reflectiveProtoTypes sync.Map

// Open generated structs without descriptor-only features can retain the table
// fast path. Oneofs, enums, explicit scalar presence and large field IDs require
// descriptors. Walk child descriptors too, including recursive schemas.
func needsProtoReflection(pm proto.Message) bool {
	t := reflect.TypeOf(pm)
	if cached, ok := reflectiveProtoTypes.Load(t); ok {
		return cached.(bool)
	}
	seen := make(map[protoreflect.FullName]bool)
	var visit func(protoreflect.MessageDescriptor) bool
	visit = func(md protoreflect.MessageDescriptor) bool {
		if seen[md.FullName()] {
			return false
		}
		seen[md.FullName()] = true
		if md.ExtensionRanges().Len() > 0 {
			return true
		}
		for i := 0; i < md.Fields().Len(); i++ {
			fd := md.Fields().Get(i)
			if fd.Number() > 65535 || fd.ContainingOneof() != nil || fd.Kind() == protoreflect.EnumKind || (fd.HasPresence() && fd.Kind() != protoreflect.MessageKind) {
				return true
			}
			if fd.Message() != nil && visit(fd.Message()) {
				return true
			}
		}
		return false
	}
	use := visit(pm.ProtoReflect().Descriptor())
	// Opaque generated APIs expose no protobuf-tagged fields to the table codec.
	if !use {
		st := t
		if st.Kind() == reflect.Pointer {
			st = st.Elem()
		}
		hasFields := false
		if st.Kind() == reflect.Struct {
			for i := 0; i < st.NumField(); i++ {
				if st.Field(i).Tag.Get("protobuf") != "" {
					hasFields = true
					break
				}
			}
		}
		use = !hasFields
	}
	reflectiveProtoTypes.Store(t, use)
	return use
}
