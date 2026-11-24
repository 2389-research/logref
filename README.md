# logref

A log line, and an error that is only logged, must name an artifact, a file, a person or a machine only through a
ref. With private logs on, the ref shows a short keyed hash in place of the name, so whoever reads the machine's
logs learns no name while the lines still read together.

`logref` is that ref, and `logref/guardtest` is the test that holds a repository to the rule. It was lifted out of
[artifact-host](https://github.com/2389-research/artifacts), where the rule is enforced on every commit.

## The rule

- A log line shows a name through `ref`: the name itself, or `#` and eight hex digits when private logs are on.
- The digits are an HMAC-SHA256 under a key the host keeps, over `"log-ref\x00"` + the name. One key keeps one
  name's digits stable, so the older lines about an artifact still read together with the newer ones. The key
  never reaches the log, so its reader cannot match the digits to a guessed name.
- An error that is only logged is a log line too. An error a caller reads and returns is not.
- The guard reads every package's source, not one package's, so a log line in a new package cannot skip the rule
  unseen. A line that names nothing a person could look up (a kind, a state, a constant, an opaque id) carries
  `// log-name-free: <why>` on the call's own line, with the reason, so the allowlist stays visible.

## Adopt it

```sh
go get github.com/2389-research/logref
```

One ref per process, built where the process is configured, and every log line that names something through it:

```go
ref := logref.New(key, privateLogs) // key is a secret the process keeps

s.logf("deleted %s by %s", ref.Name(slug), ref.Name(login))
```

Then let the guard read the repository. It parses the source, so it needs no build, and it runs in `go test`:

```go
// guardtest_test.go
package mypkg_test

import (
	"path/filepath"
	"testing"

	"github.com/2389-research/logref/guardtest"
)

func TestLogCallsWrapNames(t *testing.T) {
	guardtest.Check(t, guardtest.Config{
		Root: filepath.Join("..", ".."), // the module root
		// Prefixes whose output goes to whoever ran the command, not into the log.
		OutOfRule: []string{"internal/cli/"},
		// Floors, well under today's count: a walk that silently skips a package that logs would
		// otherwise pass by reading nothing.
		AtLeastByDir: map[string]int{"internal/server": 80},
		AtLeastCalls: 100,
	})
}
```

A package that writes its log lines through a logger passed in as an argument (`logf`, `logF`, `printf`) is read
too, since the names are in the call's arguments. A name wrapped as `s.ref(x)`, `ref.Name(x)` or `s.logRef.Name(x)`
is accepted as it is.

## Licence

MIT. See `LICENSE`.
