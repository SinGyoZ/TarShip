package proto

import (
	"encoding/json"
	"testing"
)

func TestPeekType(t *testing.T) {
	data, err := json.Marshal(Hello{Type: TypeHello, AgentVersion: "0.1.0", Hostname: "vps-01", ProtocolVersion: Version})
	if err != nil {
		t.Fatal(err)
	}
	typ, err := PeekType(data)
	if err != nil || typ != TypeHello {
		t.Fatalf("PeekType = %q, %v; want %q", typ, err, TypeHello)
	}
	if _, err := PeekType([]byte("not json")); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

func TestHelloRoundTrip(t *testing.T) {
	in := Hello{Type: TypeHello, AgentVersion: "1.2.3", Hostname: "h", ProtocolVersion: 1}
	data, _ := json.Marshal(in)
	var out Hello
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip = %+v; want %+v", out, in)
	}
}
