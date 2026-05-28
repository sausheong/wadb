package cursor

import "testing"

func TestCursor_RoundTrip(t *testing.T) {
	c := Cursor{Ts: 1716800000, ID: "ABCDEF12345"}
	enc := c.Encode()
	if enc == "" {
		t.Fatal("encoded empty")
	}
	got, err := Decode(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != c {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, c)
	}
}

func TestDecode_EmptyReturnsZeroNoError(t *testing.T) {
	got, err := Decode("")
	if err != nil {
		t.Errorf("empty cursor returned error: %v", err)
	}
	if got != (Cursor{}) {
		t.Errorf("empty cursor decoded to %+v, want zero value", got)
	}
}

func TestDecode_GarbageReturnsError(t *testing.T) {
	if _, err := Decode("not-base64!!"); err == nil {
		t.Error("expected error for invalid cursor")
	}
}
