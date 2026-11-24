// ABOUTME: Holds the guard itself to a planted file, so a widened matcher or a skip list cannot turn it into a test
// ABOUTME: that passes always, and holds the camel-case splitter to names it must see.
package guardtest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The guard is only worth its list if it fails on a bare name, so this holds it to a planted file: the lines it
// must catch, and the ones it must leave alone.
func TestCheckFailsOnAPlantedBareName(t *testing.T) {
	dir := t.TempDir()
	src := `package planted

func f(s *S, slug, login, path, name string, rows int, backupURL string) {
	s.logf("delete %s by %s", slug, login)
	s.logf("delete %s by %s", s.ref(slug), s.ref(login))
	s.logf("private logs: on. Each artifact, file, login and machine shows as # and eight hex digits")
	s.logf("host.%s, a %s call, ran %v", name, "read", 3)
	s.logf("read %s", path) // log-name-free: the path of this file's own template
	s.logf(fmt.Sprintf("delete %s", slug))
	s.logf("%s", s.logRef.Name(login))
	s.logf("wrote %d rows", rows)
	s.logf("saved to %s", backupURL)
}
`
	if err := os.WriteFile(filepath.Join(dir, "planted.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	scan, err := Walk(Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Checked != 9 {
		t.Fatalf("read %d log calls of the 9 planted; the walk misses the ones it must read", scan.Checked)
	}
	var got []string
	for _, problem := range scan.Problems {
		got = append(got, problem.Arg)
	}
	// backupURL is the one that needs the camel case split: a name ending in a word still names things.
	want := []string{"slug", "login", "name", `fmt.Sprintf("delete %s", slug)`, "backupURL"}
	if !slices.Equal(got, want) {
		t.Fatalf("flagged %q, want %q", got, want)
	}
}

// The consumer's config, not the rule, decides which folders are read and which folder a floor counts: OutOfRule
// leaves a folder unread, and ByDir keys the calls it did read by the folder a consumer writes in AtLeastByDir.
func TestWalkReadsTheConfigsFolders(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "quiet"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package quiet

func f(s *S, slug string) {
	s.logf("delete %s", slug)
}
`
	if err := os.WriteFile(filepath.Join(dir, "internal", "quiet", "quiet.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	scan, err := Walk(Config{Root: dir, OutOfRule: []string{"internal/quiet/"}})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Checked != 0 || len(scan.Problems) != 0 {
		t.Fatalf("a folder out of the rule was read: %d calls, %q", scan.Checked, scan.Problems)
	}
	scan, err = Walk(Config{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Checked != 1 || scan.ByDir["internal/quiet"] != 1 {
		t.Fatalf("byDir = %v, checked = %d, want one call in internal/quiet", scan.ByDir, scan.Checked)
	}
	if len(scan.Problems) != 1 {
		t.Fatalf("problems = %q, want the bare slug flagged", scan.Problems)
	}
}

// The splitter must split names the matcher can read, and keep words whole: a capital run of two or more ends at
// its last capital (HTTPServer is HTTP and Server), and a single capital at a word's head is that word's own
// (Health). HealthFileName was split wrong once, and the guard went blind to a line it was built for.
func TestLogCallWordsSplitsNamesWhole(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"HealthFileName", "Health File Name"},
		{"backupURL", "backup URL"},
		{"remoteAddr", "remote Addr"},
		{"hostName", "host Name"},
		{"HTTPServer", "HTTP Server"},
		{"Health", "Health"},
		{"s.ref", "s.ref"},
	} {
		if got := logCallWords(tc.name); got != tc.want {
			t.Errorf("%s split as %q, want %q", tc.name, got, tc.want)
		}
	}
}
