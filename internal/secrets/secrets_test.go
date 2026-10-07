package secrets

import (
	"reflect"
	"testing"
)

func TestParseAndSerialize(t *testing.T) {
	in := []byte("# comment\nDATABASE_URL=postgres://x\n\nexport STRIPE_KEY=sk_123\nQUOTED=\"a b c\"\n")
	s, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 3 {
		t.Fatalf("len = %d, want 3", s.Len())
	}
	if v, _ := s.Get("DATABASE_URL"); v != "postgres://x" {
		t.Fatalf("DATABASE_URL = %q", v)
	}
	if v, _ := s.Get("STRIPE_KEY"); v != "sk_123" {
		t.Fatalf("STRIPE_KEY = %q", v)
	}
	if v, _ := s.Get("QUOTED"); v != "a b c" {
		t.Fatalf("QUOTED = %q", v)
	}

	// Serialize is sorted and deterministic.
	out := string(s.Serialize())
	want := "DATABASE_URL=postgres://x\nQUOTED=a b c\nSTRIPE_KEY=sk_123\n"
	if out != want {
		t.Fatalf("serialize =\n%q\nwant\n%q", out, want)
	}
}

func TestSetUpdateAndDelete(t *testing.T) {
	s := New()
	s.Set("A", "1")
	s.Set("B", "2")
	s.Set("A", "3") // update
	if v, _ := s.Get("A"); v != "3" {
		t.Fatalf("A = %q, want 3", v)
	}
	if !s.Delete("B") {
		t.Fatal("delete B returned false")
	}
	if s.Delete("B") {
		t.Fatal("second delete B returned true")
	}
	if !reflect.DeepEqual(s.Keys(), []string{"A"}) {
		t.Fatalf("keys = %v", s.Keys())
	}
}

func TestEnvLines(t *testing.T) {
	s := New()
	s.Set("Z", "last")
	s.Set("A", "first")
	got := s.EnvLines()
	want := []string{"A=first", "Z=last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnvLines = %v, want %v", got, want)
	}
}

func TestParseMissingEquals(t *testing.T) {
	if _, err := Parse([]byte("NOEQUALS\n")); err == nil {
		t.Fatal("expected error for line without '='")
	}
}
