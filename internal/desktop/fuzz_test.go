package desktop

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// unescapeValue undoes the string escapes of a Desktop Entry value
// (\s \n \t \r \\), as a launcher reads them before the Exec rules.
func unescapeValue(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// parseExec splits an Exec value by the specification: arguments are
// separated by spaces, may be double-quoted, and inside quotes a
// backslash escapes " ` $ \; %% is a literal %, any other %x a field
// code (none is expected here).
func parseExec(t *testing.T, s string) []string {
	var args []string
	var cur strings.Builder
	inQuotes, started := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuotes && c == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\", s[i+1]) >= 0:
			i++
			cur.WriteByte(s[i])
		case c == '"':
			inQuotes = !inQuotes
			started = true
		case c == '%':
			if i+1 < len(s) && s[i+1] == '%' {
				i++
				cur.WriteByte('%')
			} else {
				t.Fatalf("field code in %q", s)
			}
		case c == ' ' && !inQuotes:
			if started || cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
		}
	}
	if inQuotes {
		t.Fatalf("unterminated quote in %q", s)
	}
	if started || cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// Whatever an environment is called, its launcher entry must run exactly
// `omavm open NAME`: a name that breaks out of its quotes would run
// something else, or open another environment.
func FuzzExecLineRoundTrip(f *testing.F) {
	for _, seed := range []string{"Fedora", `a"b`, "c`d", "$HOME", `back\slash`, "50% off", "%u", "two  spaces", "new\nline", "tab\there", ` lead`, `\"`, `\\"`, "Ação 🙂"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		// Names are UTF-8 text: core refuses anything else.
		if !utf8.ValidString(name) {
			return
		}
		args := []string{"/opt/omavm/omavm", "open", name}
		line := escapeValue(execLine(args))
		if strings.ContainsAny(line, "\n\r") {
			t.Fatalf("Exec value spans lines: %q", line)
		}
		got := parseExec(t, unescapeValue(line))
		if !slices.Equal(got, args) {
			t.Fatalf("name %q: Exec %q runs %q", name, line, got)
		}
	})
}
