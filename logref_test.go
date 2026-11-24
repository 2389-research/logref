// ABOUTME: Tests that a ref hides a name with private logs on, gives one name one value on one host, and keeps
// ABOUTME: the value it had before this package was lifted out of internal/server, so old logs still read together.
package logref

import (
	"strings"
	"testing"
)

// testKey is the key a host keeps. Every test uses it, so a golden value below pins the derivation.
var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestNameIsTheNameWithoutPrivateLogs(t *testing.T) {
	for _, name := range []string{"merger-plan", "pat@customer.com", "artifacts.example.net"} {
		if got := New(testKey, false).Name(name); got != name {
			t.Errorf("without private logs Name(%q) = %q, want the name", name, got)
		}
	}
}

func TestNameIsStableAndKeyed(t *testing.T) {
	r := New(testKey, true)
	first, again := r.Name("merger-plan"), r.Name("merger-plan")
	if first != again || first == r.Name("merger-notes") {
		t.Fatalf("one host must give one name one ref, and two names two refs: %q, %q", first, again)
	}
	other := New([]byte("fedcba9876543210fedcba9876543210"), true)
	if other.Name("merger-plan") == first {
		t.Fatal("two keys gave one name the same ref: the ref is not keyed")
	}
}

// A name shows as a hash of eight hex digits after "#": short enough to read, and never the name.
func TestNameIsShortHex(t *testing.T) {
	got := New(testKey, true).Name("merger-plan")
	if !strings.HasPrefix(got, "#") || len(got) != 9 || strings.Contains(got, "merger") {
		t.Fatalf("Name = %q, want # and eight hex digits", got)
	}
	for _, c := range got[1:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("Name = %q, want # and eight hex digits", got)
		}
	}
}

// The derivation is the one the package shipped with: an HMAC-SHA256 under the key over "log-ref\x00" + the
// name. A host that keeps its key keeps the digits in its older log lines, so the two can be read together.
func TestNameKeepsTheDerivation(t *testing.T) {
	for name, want := range map[string]string{
		"merger-plan":                  "#dafab450",
		"pat@customer.com":             "#dd8c248d",
		"artifacts.folk-fujita.ts.net": "#b4536262",
	} {
		if got := New(testKey, true).Name(name); got != want {
			t.Errorf("Name(%q) = %q, want %q", name, got, want)
		}
	}
}
