package secret

import "testing"

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := NewBox("app-secret")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("sk-123")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := box.Open(sealed); err != nil || got != "sk-123" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestOpenRejectsWrongSecretAndGarbage(t *testing.T) {
	a, _ := NewBox("a")
	b, _ := NewBox("b")
	sealed, _ := a.Seal("sk-123")
	if _, err := b.Open(sealed); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, err := a.Open([]byte("x")); err == nil {
		t.Fatal("short value accepted")
	}
}
