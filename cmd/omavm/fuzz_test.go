package main

import (
	"slices"
	"strings"
	"testing"
)

// extractValueFlag takes --label VALUE / --label=VALUE out of any position.
// Checked against a direct reading of the same rule: what remains is every
// other argument, in order, and the value is the last one given.
func FuzzExtractValueFlag(f *testing.F) {
	f.Add("snapshot\x00create\x00vm\x00--label\x00before upgrade")
	f.Add("--label=x\x00vm\x00--label\x00y")
	f.Add("vm\x00--label")
	f.Add("--label\x00--label\x00vm")
	f.Add("--labelx=1\x00--label=\x00vm")
	f.Fuzz(func(t *testing.T, joined string) {
		args := strings.Split(joined, "\x00")
		rest, value, err := extractValueFlag(args, "label")

		var wantRest []string
		wantValue, missing := "", false
		for i := 0; i < len(args); i++ {
			switch a := args[i]; {
			case strings.HasPrefix(a, "--label="):
				wantValue = strings.TrimPrefix(a, "--label=")
			case a == "--label":
				if i+1 == len(args) {
					missing = true
				} else {
					wantValue = args[i+1]
					i++
				}
			default:
				wantRest = append(wantRest, a)
			}
		}
		if missing {
			if err == nil {
				t.Fatalf("%q: a trailing --label without a value was accepted", args)
			}
			return
		}
		if err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		if value != wantValue || !slices.Equal(rest, wantRest) && !(len(rest) == 0 && len(wantRest) == 0) {
			t.Fatalf("%q: got %q %q, want %q %q", args, rest, value, wantRest, wantValue)
		}
	})
}
