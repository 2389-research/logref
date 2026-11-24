// ABOUTME: How a log line names an artifact, a file or a person. With private logs on, it shows a short keyed
// ABOUTME: hash in place of the name, so whoever reads the machine's logs learns no name.
package logref

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Ref turns a name into the form a log line may show it in: the name itself, or with private logs a short
// keyed hash. It is keyed by a secret the caller keeps, so the digits cannot be matched to a guessed name from
// the logs alone.
type Ref struct {
	key     []byte
	private bool
}

// New returns the ref a host's log lines use. key is the secret the host keeps, so that one name gives one value
// for as long as the host keeps it: lines about one artifact still read together. private says whether names are
// kept out of the log.
func New(key []byte, private bool) *Ref {
	return &Ref{key: key, private: private}
}

// Private says whether names are kept out of the log at all. A caller that chooses a logger, rather than a single
// line, reads it: a third party's log stream may not be ours to redact.
func (r *Ref) Private() bool {
	return r.private
}

// Name returns name as a log line may show it. Every log argument that names an artifact, a file, a person or
// a machine goes through it (guardtest.Check). With private logs it returns "#" and eight hex digits.
func (r *Ref) Name(name string) string {
	if !r.private {
		return name
	}
	mac := hmac.New(sha256.New, r.key)
	// The prefix keeps this hash apart from every other use of the key, such as a page token.
	mac.Write([]byte("log-ref\x00" + name))
	return "#" + hex.EncodeToString(mac.Sum(nil)[:4])
}
