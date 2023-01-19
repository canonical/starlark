package startest_test

import (
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
	src:    "a\nb",
	result: "a\nb",
}, {
	src:    "\ta\n\tb",
	result: "a\nb",
}, {
	src:    "    a\n    b",
	result: "a\nb",
}, {
	src:    "a\n\tb\nc",
	result: "a\n\tb\nc",
}, {
	src:    "a\n  b\nc",
	result: "a\n  b\nc",
}, {
	src:    "\ta\n\t\tb\n\tc",
	result: "a\n\tb\nc",
}, {
	src:    "  a\n    b\n  c",
	result: "a\n  b\nc",
}, {
	src:    "    a\n    \tb\n    c",
	result: "a\n\tb\nc",
}, {
	src:    "\t  a\n\t    b\n\t  c",
	result: "a\n  b\nc",
}, {
	src:    "\ta\nb",
	expect: `Invalid indentation on line 2: expected line starting "\t" but got "b"`,
}}

func TestReindent(t *testing.T) {
	for _, test := range reindentTests {
		testReindent(t, test)
	}
}

func testReindent(t *testing.T, test reindentTest) {
	reindented, err := startest.Reindent(test.src)
	if test.expect != "" {
		if err == nil {
			t.Errorf("%#v: expected error", test.src)
		} else if msg := err.Error(); msg != test.expect {
			t.Errorf("%#v: unexpected error: expected '%v' but got '%v'", test.src, test.expect, err)
		}
	} else if string(reindented) != test.result && string(reindented) != test.result+"\n" {
		t.Errorf("%#v: incorrect output: expected %#v but got %#v", test.src, test.result, reindented)
	}
}
