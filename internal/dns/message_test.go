package dns

import (
	"encoding/hex"
	"testing"
)

func TestParseRealQuery(t *testing.T) {
	raw, _ := hex.DecodeString(
		"b96201000001000000000000" +
			"06676f6f676c65" +
			"03636f6d00" +
			"0001" +
			"0001",
	)

	m, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(m.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(m.Questions))
	}
	q := m.Questions[0]
	if q.Name != "google.com" {
		t.Errorf("expected google.com, got %q", q.Name)
	}
	if q.Type != TypeA {
		t.Errorf("expected type A, got %d", q.Type)
	}
	t.Logf("parsed: %s type=%d class=%d", q.Name, q.Type, q.Class)
}

func TestPackRoundtrip(t *testing.T) {
	m := &Message{}
	m.ID = 0x1234
	m.SetRD(true)
	m.Questions = []Question{{Name: "example.com", Type: TypeA, Class: ClassIN}}

	packed, err := m.Pack()
	if err != nil {
		t.Fatalf("pack error: %v", err)
	}
	m2, err := ParseMessage(packed)
	if err != nil {
		t.Fatalf("reparse error: %v", err)
	}
	if m2.Questions[0].Name != "example.com" {
		t.Errorf("roundtrip name mismatch: got %q", m2.Questions[0].Name)
	}
	if !m2.RD() {
		t.Error("RD bit lost in roundtrip")
	}
	t.Logf("roundtrip OK, %d bytes", len(packed))
}
