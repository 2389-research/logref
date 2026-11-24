// ABOUTME: The guard test a consumer runs to hold its log lines to the rule: no log argument names an artifact, a
// ABOUTME: file, a person or a machine unless it goes through a ref. The consumer supplies the walk and the floors.
package guardtest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// logCallNames matches an argument that names an artifact, a file, a person or a machine. It matches the text of
// the expression, so it sees a field (a.Slug), a call (u.Host) and a plain parameter (path) alike.
var logCallNames = regexp.MustCompile(`(?i)\b(slug|login|alias|node|account|path|dir|file|email|host|hostname|addr|remote|who|user|owner|peer|title|url|name|by)\b|PathValue`)

// logCallSelectors are the selector names that write to a log: the standard logger's, and a logger's own logf
// methods. Errorf is here because a logger may have one; Error is not, since http.Error writes a response and
// t.Error a test. fmt.Errorf and errors.New are left out by writesToLog below: they build a value, they do not
// write.
var logCallSelectors = []string{"Logf", "logf", "Printf", "Println", "Print", "Fatal", "Fatalf", "Errorf", "Warn", "Info"}

// passedInLogs are the logger names a package takes as an argument and calls directly, as a package does with its
// logf: no receiver, so the selector list above cannot see them.
var passedInLogs = []string{"logf", "logF", "printf"}

// NameFreeComment marks a log call whose arguments name nothing a person could look up: a kind, a state, a
// constant, an opaque id. It goes on the call's own line and needs a reason, so the allowlist is visible instead
// of accidental.
const NameFreeComment = "// log-name-free:"

// Config is the walk a consumer supplies. The rule is this package's; which folders it reads, whose output is not
// a log, and how much source a silent walk must have read are the consumer's.
type Config struct {
	// Root is the folder to read, usually the consumer's module root.
	Root string
	// OutOfRule are the path prefixes under Root whose output goes to whoever ran a command, not into a log:
	// a command that prints a name to a terminal is read by its caller, not by the log's reader.
	OutOfRule []string
	// AtLeastByDir is a floor per folder, keyed as firstTwoFolders writes it ("internal/server"): a walk that
	// silently skips a package that logs would otherwise pass by reading nothing.
	AtLeastByDir map[string]int
	// AtLeastCalls is a floor for the whole walk, for the same reason.
	AtLeastCalls int
}

// Problem is one log argument that names something without going through a ref.
type Problem struct {
	Pos token.Position // where the argument is
	Arg string         // the argument's source
}

func (p Problem) String() string {
	return fmt.Sprintf("%s: the log argument %s names something; wrap it in ref(...), or mark the line with %s <why>", p.Pos, p.Arg, NameFreeComment)
}

// Result is what a walk of the source found: the log calls it read, per folder, and the arguments that name
// something without going through a ref.
type Result struct {
	Checked  int            // log calls read
	ByDir    map[string]int // log calls read, per folder
	Problems []Problem
}

// Check is the guard test: it fails t on a log argument that names something without going through a ref, and on a
// walk that read too little of the source for its silence to mean anything. Put it in a _test.go file of the
// module it guards, with that module's own walk.
func Check(t testing.TB, cfg Config) {
	t.Helper()
	scan, err := Walk(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range scan.Problems {
		t.Error(problem)
	}
	for dir, least := range cfg.AtLeastByDir {
		if got := scan.ByDir[dir]; got < least {
			t.Errorf("read %d log calls in %s, want at least %d: the walk skipped the package", got, dir, least)
		}
	}
	if scan.Checked < cfg.AtLeastCalls {
		t.Errorf("read only %d log calls in the whole repository, want at least %d: the walk no longer finds the source it should", scan.Checked, cfg.AtLeastCalls)
	}
}

// Walk reads every non-test .go file under cfg.Root and returns the log arguments that name something. It parses
// only, so the source need not build.
func Walk(cfg Config) (Result, error) {
	scan := Result{ByDir: map[string]int{}}
	fset := token.NewFileSet()
	err := filepath.WalkDir(cfg.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// .git, .dashboard and the like hold no source of the program's.
			if path != cfg.Root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(cfg.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, skip := range cfg.OutOfRule {
			if strings.HasPrefix(rel, skip) {
				return nil
			}
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return err
		}
		before := scan.Checked
		scan.read(fset, file, strings.Split(string(src), "\n"))
		// The folder says how far the walk got: a walk that stops at the repository root reads no package.
		if dir := firstTwoFolders(rel); scan.Checked > before {
			scan.ByDir[dir] += scan.Checked - before
		}
		return nil
	})
	return scan, err
}

// firstTwoFolders gives the folder a path belongs to, as deep as a module's own packages go: "internal/server",
// "cmd/tool", "e2e".
func firstTwoFolders(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return parts[0]
	}
	return strings.Join(parts[:2], "/")
}

// read adds the log calls of one file to the scan.
func (s *Result) read(fset *token.FileSet, file *ast.File, lines []string) {
	text := func(n ast.Node) string {
		var buf bytes.Buffer
		printer.Fprint(&buf, fset, n)
		return buf.String()
	}
	// nameFree says whether the call carries the opt-out. It is read on the call's own line only: a marker above
	// would also silence the next call, which is exactly the line a change is most likely to add.
	nameFree := func(pos token.Position) bool {
		return pos.Line >= 1 && pos.Line <= len(lines) && strings.Contains(lines[pos.Line-1], NameFreeComment)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			// logf and printf are loggers passed in from another package: the line is written where they are
			// called, but the names are here, in the arguments.
			if ident, isIdent := call.Fun.(*ast.Ident); isIdent && slices.Contains(passedInLogs, ident.Name) {
				s.Checked++
				s.args(fset, call, nameFree(fset.Position(call.Pos())), text)
			}
			return true
		}
		if !writesToLog(sel) {
			return true
		}
		s.Checked++
		s.args(fset, call, nameFree(fset.Position(call.Pos())), text)
		return true
	})
}

// args records every argument of one log call that names something without going through a ref.
func (s *Result) args(fset *token.FileSet, call *ast.CallExpr, nameFree bool, text func(ast.Node) string) {
	if nameFree {
		return
	}
	// A literal first argument is the line's own wording, not a value: it may say "file" or "login" as words.
	// Anything else in that place is a message built before the call, so it is read like every other argument.
	for i, arg := range call.Args {
		if _, literal := arg.(*ast.BasicLit); i == 0 && literal {
			continue
		}
		if isRefd(arg) {
			continue
		}
		if argText := text(arg); logCallNames.MatchString(logCallWords(argText)) {
			s.Problems = append(s.Problems, Problem{Pos: fset.Position(arg.Pos()), Arg: argText})
		}
	}
}

// logCallWords puts spaces inside a Go name where its words begin, so that a name ending in a word is still seen:
// backupURL is a URL, remoteAddr an address, hostName a host. Go names run their words together, and the matcher
// above reads whole words only. Two boundaries matter: a lower-to-upper step starts a new word (backupURL), and
// the end of a capital RUN of two or more starts one too (HTTPServer is HTTP and Server; the split goes before
// the run's last capital). A single capital at a word's head is no boundary of its own (Health stays one word).
func logCallWords(text string) string {
	rs := []rune(text)
	boundary := make([]bool, len(rs)+1)
	for i := 1; i < len(rs); i++ {
		prev, r := rs[i-1], rs[i]
		switch {
		case (unicode.IsLower(prev) || unicode.IsDigit(prev)) && unicode.IsUpper(r):
			boundary[i] = true // backupURL: U starts a word
		case unicode.IsUpper(prev) && unicode.IsLower(r):
			// A capital run ends here if another capital sits before prev. The split goes before the run's last
			// capital — that is, before prev — so mark the boundary one rune back and undo the one this would
			// otherwise place inside the run.
			if i >= 2 && unicode.IsUpper(rs[i-2]) {
				boundary[i-1] = true // HTTPServer: S ends the run, the word Server starts at S
			}
		}
	}
	var out strings.Builder
	for i, r := range rs {
		if boundary[i] {
			out.WriteByte(' ')
		}
		out.WriteRune(r)
	}
	return strings.ReplaceAll(out.String(), "_", " ")
}

// writesToLog says whether a call writes its arguments to a log, as opposed to building a value that a caller may
// log or return. fmt.Errorf and errors.New are how nearly every error text is built, and most of those name the
// file or the folder they were about, on purpose: an error a caller reads is not a log line.
func writesToLog(sel *ast.SelectorExpr) bool {
	if !slices.Contains(logCallSelectors, sel.Sel.Name) {
		return false
	}
	var receiver bytes.Buffer
	printer.Fprint(&receiver, token.NewFileSet(), sel.X)
	switch receiver.String() {
	case "fmt", "errors":
		return false
	}
	return true
}

// isRefd says whether an argument already goes through a ref: s.ref(name), d.ref(name), ref.Name(name),
// s.logRef.Name(name).
func isRefd(expr ast.Expr) bool {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return isRefd(paren.X)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name == "ref" {
		return true
	}
	// A *logref.Ref shows a name through Name. Its receiver is a field or a variable named ...ref.
	if sel.Sel.Name != "Name" {
		return false
	}
	var receiver bytes.Buffer
	printer.Fprint(&receiver, token.NewFileSet(), sel.X)
	return strings.HasSuffix(strings.ToLower(receiver.String()), "ref")
}
