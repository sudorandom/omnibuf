package main

import (
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
	req.ProtoFile[0].MessageType = []*descriptorpb.DescriptorProto{{Name: proto.String("Wide"), Field: []*descriptorpb.FieldDescriptorProto{
		{Name: proto.String("flag"), Number: proto.Int32(65535), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()},
		{Name: proto.String("numbers"), Number: proto.Int32(65), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()},
	}}}
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
	const testSource = `package pb
import("testing";"reflect";"github.com/sudorandom/omnibuf/wire")
type Wide struct { Flag bool ` + "`omnibuf:\"65535\"`" + `;Numbers []uint32 ` + "`omnibuf:\"65\"`" + ` }
func TestRoundTrip(t *testing.T){
 for _, flag:=range []bool{true,false}{
 in:=Wide{flag,[]uint32{1,2}};packet:=in.MarshalOmniBufAppend(nil)
 got,ok:=wire.ReadBool(packet,65535);if !ok || got!=flag {t.Fatalf("boolean tag: %x",packet)}
 var out Wide;if err:=out.UnmarshalOmniBuf(append(packet,packet...),false);err!=nil{t.Fatal(err)}
 if out.Flag!=flag || !reflect.DeepEqual(out.Numbers,[]uint32{1,2,1,2}) {t.Fatalf("merge: %+v",out)}
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
