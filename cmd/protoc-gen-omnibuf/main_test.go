package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestGeneratedWideTags(t *testing.T) {
	req := peerProtoRequest()
	var fields []*descriptorpb.FieldDescriptorProto
	for i := 1; i <= 66; i++ {
		fields = append(fields, &descriptorpb.FieldDescriptorProto{
			Name:   proto.String(fmt.Sprintf("f_%d", i)),
			Number: proto.Int32(int32(i)),
			Type:   descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(),
		})
	}
	fields = append(fields,
		&descriptorpb.FieldDescriptorProto{Name: proto.String("flag"), Number: proto.Int32(65535), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
		&descriptorpb.FieldDescriptorProto{Name: proto.String("numbers"), Number: proto.Int32(70), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
		&descriptorpb.FieldDescriptorProto{Name: proto.String("fixed_nums"), Number: proto.Int32(71), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
	)
	req.ProtoFile[0].MessageType = []*descriptorpb.DescriptorProto{{Name: proto.String("Wide"), Field: fields}}
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatal(err)
	}
	generateFile(gen, gen.Files[0])
	resp := gen.Response()
	if resp.GetError() != "" {
		t.Fatal(resp.GetError())
	}
	dir := t.TempDir()
	generated := filepath.Join(dir, "wide.go")
	testFile := filepath.Join(dir, "wide_test.go")
	if err := os.WriteFile(generated, []byte(resp.File[0].GetContent()), 0600); err != nil {
		t.Fatal(err)
	}
	var structFields strings.Builder
	for i := 1; i <= 66; i++ {
		fmt.Fprintf(&structFields, "\tF_%d uint32 `omnibuf:\"%d\"`\n", i, i)
	}
	structFields.WriteString("\tFlag bool `omnibuf:\"65535\"`\n\tNumbers []uint32 `omnibuf:\"70\"`\n\tFixedNums []uint32 `omnibuf:\"71,wire=fixed32\"`\n")
	testSource := `package pb
import("testing";"reflect";"github.com/sudorandom/omnibuf/wire")
type Wide struct {
` + structFields.String() + `}
func TestRoundTrip(t *testing.T){
 for _, flag:=range []bool{true,false}{
 in:=Wide{F_1: 11, F_66: 66, Flag: flag, Numbers: []uint32{1,2}, FixedNums: []uint32{7,8}};packet:=in.MarshalOmniBufAppend(nil)
 got,ok:=wire.ReadBool(packet,65535);if !ok || got!=flag {t.Fatalf("boolean tag: %x",packet)}
 var out Wide;if err:=out.UnmarshalOmniBuf(append(packet,packet...),false);err!=nil{t.Fatal(err)}
 if out.F_1!=11 || out.F_66!=66 || out.Flag!=flag || !reflect.DeepEqual(out.Numbers,[]uint32{1,2,1,2}) || !reflect.DeepEqual(out.FixedNums,[]uint32{7,8,7,8}) {t.Fatalf("merge: %+v",out)}
 if err:=out.UnmarshalOmniBuf(nil,false);err!=nil{t.Fatal(err)}
 if out.F_1!=0 || out.F_66!=0 || out.Flag || len(out.Numbers)!=0 || len(out.FixedNums)!=0 {t.Fatalf("clearUnseen >64 failed: %+v",out)}
 badArr := wire.AppendVarint(nil, (uint64(71)<<4)|uint64(wire.WireArray))
 badArr = wire.EncodeFloat32Arr(badArr, []float32{1.0})
 if err:=out.UnmarshalOmniBuf(badArr,false);err==nil{t.Fatal("accepted float32 array for fixed32 repeated field")}
 }
}`
	if err := os.WriteFile(testFile, []byte(testSource), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "test", generated, testFile).CombinedOutput(); err != nil {
		t.Fatalf("generated code failed: %v\n%s", err, out)
	}
}
func TestRejectUnsupportedGeneratedField(t *testing.T) {
	req := peerProtoRequest()
	req.ProtoFile[0].MessageType[0].Field[0].Proto3Optional = proto.Bool(true)
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatal(err)
	}
	generateFile(gen, gen.Files[0])
	resp := gen.Response()
	if !strings.Contains(resp.GetError(), "unsupported field") {
		t.Fatalf("missing actionable diagnostic: %v", resp)
	}
}
