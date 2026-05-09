package dns

import (
	"testing"
)

func TestParseA(t *testing.T) {
	ip, err := ParseA([]byte{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("ParseA: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("ParseA = %q, want 1.2.3.4", ip)
	}
}

func TestParseAZero(t *testing.T) {
	ip, err := ParseA([]byte{0, 0, 0, 0})
	if err != nil {
		t.Fatalf("ParseA: %v", err)
	}
	if ip != "0.0.0.0" {
		t.Errorf("ParseA = %q, want 0.0.0.0", ip)
	}
}

func TestParseABroadcast(t *testing.T) {
	ip, err := ParseA([]byte{255, 255, 255, 255})
	if err != nil {
		t.Fatalf("ParseA: %v", err)
	}
	if ip != "255.255.255.255" {
		t.Errorf("ParseA = %q, want 255.255.255.255", ip)
	}
}

func TestParseATooShort(t *testing.T) {
	_, err := ParseA([]byte{1, 2})
	if err == nil {
		t.Error("expected error for data shorter than 4 bytes")
	}
}

func TestParseATooLong(t *testing.T) {
	_, err := ParseA([]byte{1, 2, 3, 4, 5})
	if err == nil {
		t.Error("expected error for data longer than 4 bytes")
	}
}

func TestParseAEmpty(t *testing.T) {
	_, err := ParseA([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestPackNameSimple(t *testing.T) {
	b := PackName("example.com")
	if len(b) == 0 {
		t.Fatal("PackName returned empty bytes")
	}
	if b[len(b)-1] != 0x00 {
		t.Error("packed name should end with null byte")
	}
	// \x07example\x03com\x00 = 13 bytes
	if len(b) != 13 {
		t.Errorf("PackName(example.com) length = %d, want 13", len(b))
	}
}

func TestPackNameEmpty(t *testing.T) {
	b := PackName("")
	if len(b) != 1 || b[0] != 0 {
		t.Errorf("PackName('') should be [0x00], got %v", b)
	}
}

func TestPackNameRoot(t *testing.T) {
	b := PackName(".")
	if len(b) != 1 || b[0] != 0 {
		t.Errorf("PackName('.') should be [0x00], got %v", b)
	}
}

func TestPackNameRoundtrip(t *testing.T) {
	for _, name := range []string{"example.com", "sub.example.com", "a.b.c.d"} {
		packed := PackName(name)
		got, _, err := ParseName(packed, 0)
		if err != nil {
			t.Fatalf("ParseName(%q): %v", name, err)
		}
		if got != name {
			t.Errorf("roundtrip(%q) = %q", name, got)
		}
	}
}

func TestParseNameOffset(t *testing.T) {
	// Build a buffer: 3 padding bytes + packed "hello.test"
	prefix := []byte{0xAA, 0xBB, 0xCC}
	packed := PackName("hello.test")
	buf := append(prefix, packed...)

	name, next, err := ParseName(buf, 3)
	if err != nil {
		t.Fatalf("ParseName with offset: %v", err)
	}
	if name != "hello.test" {
		t.Errorf("ParseName = %q, want hello.test", name)
	}
	if next != len(buf) {
		t.Errorf("next offset = %d, want %d", next, len(buf))
	}
}

func TestHeaderQRBit(t *testing.T) {
	var h Header
	if h.QR() {
		t.Error("QR should be false initially")
	}
	h.SetQR(true)
	if !h.QR() {
		t.Error("QR should be true after setting")
	}
	h.SetQR(false)
	if h.QR() {
		t.Error("QR should be false after clearing")
	}
}

func TestHeaderRDBit(t *testing.T) {
	var h Header
	if h.RD() {
		t.Error("RD should be false initially")
	}
	h.SetRD(true)
	if !h.RD() {
		t.Error("RD should be true after setting")
	}
}

func TestHeaderRAbits(t *testing.T) {
	var h Header
	h.SetRA(true)
	if h.Flags&0x0080 == 0 {
		t.Error("RA bit (7) should be set")
	}
	h.SetRA(false)
	if h.Flags&0x0080 != 0 {
		t.Error("RA bit (7) should be cleared")
	}
}

func TestHeaderAABit(t *testing.T) {
	var h Header
	h.SetAA(true)
	if h.Flags&0x0400 == 0 {
		t.Error("AA bit (10) should be set")
	}
}

func TestHeaderRcode(t *testing.T) {
	var h Header

	if h.Rcode() != 0 {
		t.Errorf("initial rcode = %d, want 0", h.Rcode())
	}

	h.SetRcode(RcodeNXDomain)
	if h.Rcode() != RcodeNXDomain {
		t.Errorf("rcode = %d, want RcodeNXDomain (%d)", h.Rcode(), RcodeNXDomain)
	}

	h.SetRcode(RcodeServFail)
	if h.Rcode() != RcodeServFail {
		t.Errorf("rcode = %d, want RcodeServFail (%d)", h.Rcode(), RcodeServFail)
	}

	h.SetRcode(RcodeNoError)
	if h.Rcode() != 0 {
		t.Errorf("rcode = %d after clearing, want 0", h.Rcode())
	}
}

func TestHeaderRcodeDoesNotCorruptOtherBits(t *testing.T) {
	var h Header
	h.SetQR(true)
	h.SetRcode(RcodeNXDomain)

	if !h.QR() {
		t.Error("SetRcode should not clear QR bit")
	}
	if h.Rcode() != RcodeNXDomain {
		t.Errorf("rcode = %d, want %d", h.Rcode(), RcodeNXDomain)
	}
}

func TestParseMessageTooShort(t *testing.T) {
	_, err := ParseMessage([]byte{0x00, 0x01})
	if err == nil {
		t.Error("expected error for message shorter than 12 bytes")
	}
}

func TestParseMessageEmpty(t *testing.T) {
	_, err := ParseMessage([]byte{})
	if err == nil {
		t.Error("expected error for empty message")
	}
}

func TestPackWithAnswer(t *testing.T) {
	m := &Message{}
	m.ID = 0xCAFE
	m.SetQR(true)
	m.SetRA(true)
	m.Questions = []Question{{Name: "example.com", Type: TypeA, Class: ClassIN}}
	m.Answers = []RR{
		{Name: "example.com", Type: TypeA, Class: ClassIN, TTL: 300, Data: []byte{93, 184, 216, 34}},
	}

	packed, err := m.Pack()
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	got, err := ParseMessage(packed)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if got.ID != 0xCAFE {
		t.Errorf("ID = %#x, want 0xCAFE", got.ID)
	}
	if !got.QR() {
		t.Error("QR bit should be preserved through pack/parse")
	}
	if len(got.Questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(got.Questions))
	}
	if len(got.Answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(got.Answers))
	}
	if got.Answers[0].TTL != 300 {
		t.Errorf("answer TTL = %d, want 300", got.Answers[0].TTL)
	}
}

func TestTypeConstants(t *testing.T) {
	cases := map[string]uint16{
		"TypeA":     TypeA,
		"TypeNS":    TypeNS,
		"TypeCNAME": TypeCNAME,
		"TypeSOA":   TypeSOA,
		"TypeMX":    TypeMX,
		"TypeTXT":   TypeTXT,
		"TypeAAAA":  TypeAAAA,
		"ClassIN":   ClassIN,
	}
	expected := map[string]uint16{
		"TypeA":     1,
		"TypeNS":    2,
		"TypeCNAME": 5,
		"TypeSOA":   6,
		"TypeMX":    15,
		"TypeTXT":   16,
		"TypeAAAA":  28,
		"ClassIN":   1,
	}
	for name, val := range cases {
		if val != expected[name] {
			t.Errorf("%s = %d, want %d", name, val, expected[name])
		}
	}
}

func TestRcodeConstants(t *testing.T) {
	if RcodeNoError != 0 {
		t.Errorf("RcodeNoError = %d, want 0", RcodeNoError)
	}
	if RcodeServFail != 2 {
		t.Errorf("RcodeServFail = %d, want 2", RcodeServFail)
	}
	if RcodeNXDomain != 3 {
		t.Errorf("RcodeNXDomain = %d, want 3", RcodeNXDomain)
	}
}
