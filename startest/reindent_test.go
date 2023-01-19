package startest_test

import (
	"strings"
	"testing"

	"github.com/canonical/starlark/startest"
)

type reindentTest struct {
	src, result, expect string
}

var reindentTests = []reindentTest{{
	src:    "",
	result: "",
}, {
	src:    "a",
	result: "a",
}, {
	src:    "\t  a",
	result: "\t  a",
}, {
	src:    "a{}b",
	result: "a\nb",
}, {
	src:    "\ta{}\tb",
	result: "a\nb",
}, {
	src:    "    a{}    b",
	result: "a\nb",
}, {
	src:    "a{}\tb{}c",
	result: "a\n\tb\nc",
}, {
	src:    "a{}  b{}c",
	result: "a\n  b\nc",
}, {
	src:    "\ta{}\t\tb{}\tc",
	result: "a\n\tb\nc",
}, {
	src:    "  a{}    b{}  c",
	result: "a\n  b\nc",
}, {
	src:    "    a{}    \tb{}    c",
	result: "a\n\tb\nc",
}, {
	src:    "\t  a{}\t    b{}\t  c",
	result: "a\n  b\nc",
}, {
	src:    "\ta{}b",
	expect: `Invalid indentation on line 2: expected line starting "\t" but got "b"`,
}}

func TestReindent(t *testing.T) {
	for _, test := range reindentTests {
		for _, newline := range newlines {
			src := strings.ReplaceAll(test.src, "{}", newline.code)
			reindented, err := startest.Reindent(src)
			if test.expect != "" {
				if err == nil {
					t.Errorf("%#v: expected error", src)
				} else if msg := err.Error(); msg != test.expect {
					t.Errorf("%#v: unexpected error: expected '%v' but got '%v'", src, test.expect, err)
				}
			} else if string(reindented) != test.result && string(reindented) != test.result+"\n" {
				t.Errorf("%#v: incorrect output: expected %#v but got %#v", src, test.result, reindented)
			}
		}
	}
}
