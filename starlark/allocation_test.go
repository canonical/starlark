package starlark_test

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/canonical/starlark/resolve"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
)

type allocationTest struct {
	name                      string
	gen                       codeGenerator
	trend                     allocationTrend
	nSmall, nLarge            uint
	falsePositiveCancellation *regexp.Regexp
}

func (at *allocationTest) InitDefaults() {
	if at.nSmall == 0 {
		at.nSmall = 1000
	}
	if at.nLarge == 0 {
		at.nLarge = 100000
	}
}

func (at *allocationTest) IsFalsePositive(err string) bool {
	if at.falsePositiveCancellation == nil {
		return false
	}
	return at.falsePositiveCancellation.Match([]byte(err))
}

type codeGenerator func(n uint) (prog string, predecls env)

type env map[string]interface{}

// Convenience function to map common values to starlark values
func (e env) ToStarlarkPredecls() starlark.StringDict {
	predecls := make(starlark.StringDict, len(e)/2)
	for key, val := range e {
		switch val := val.(type) {
		case starlark.Value:
			predecls[key] = val
		case []starlark.Value:
			predecls[key] = starlark.NewList(val)
		case rune:
			predecls[key] = starlark.String(val)
		case string:
			predecls[key] = starlark.String(val)
		case *string:
			if val == nil {
				predecls[key] = starlark.None
				continue
			}
			predecls[key] = starlark.String(*val)
		case uint:
			predecls[key] = starlark.MakeInt(int(val))
		case int:
			predecls[key] = starlark.MakeInt(val)
		case float64:
			predecls[key] = starlark.Float(val)
		default:
			panic(fmt.Sprintf("Could not coerce %v into a starlark value", val))
		}
	}
	return predecls
}

type allocationTrend struct {
	label       string
	allocations func(n float64) float64
}

func constant(c float64) allocationTrend {
	return allocationTrend{
		label:       "remain constant",
		allocations: func(_ float64) float64 { return c },
	}
}

func linear(a float64) allocationTrend {
	return allocationTrend{
		label:       "increase linearly where f(0) =~ 0",
		allocations: func(n float64) float64 { return a * n },
	}
}

func affine(a, b float64) allocationTrend {
	return allocationTrend{
		label:       "increase linearly",
		allocations: func(n float64) float64 { return a*n + b },
	}
}

func TestPositiveDeltaDeclaration(t *testing.T) {
	thread := new(starlark.Thread)
	thread.SetMaxAllocations(0)

	// Size increases stored
	const sizeIncrease = 1000
	allocs0 := thread.Allocations()
	err := thread.DeclareSizeIncrease(sizeIncrease, "TestPositiveDeltaDeclaration")
	if err != nil {
		t.Errorf("Unexpected cancellation: %v", err)
	}
	delta := thread.Allocations() - allocs0
	if delta != sizeIncrease {
		t.Errorf("Incorrect size increase: expected %d but got %d", sizeIncrease, delta)
	}

	// Large size increase caught
	thread.SetMaxAllocations(uintptr(sizeIncrease * 1.5))
	err = thread.DeclareSizeIncrease(sizeIncrease, "TestPositiveDeltaDeclaration")
	if err == nil {
		t.Errorf("Expected allocation failure!")
	}
}

func TestNegativeDeltaAllocation(t *testing.T) {
	thread := new(starlark.Thread)
	thread.SetMaxAllocations(0)

	const maxAllocs = 100
	const minAllocs = 10
	if err := thread.DeclareSizeIncrease(100, "test-negative-deltas"); err != nil {
		t.Errorf("Unexpected error when declaring size increase: %v", err)
	}
	if maxAssignedAllocs := thread.Allocations(); maxAssignedAllocs != maxAllocs {
		t.Errorf("Failed to declare max allocations: expected %d but got %d", maxAllocs, maxAssignedAllocs)
	}
	thread.DeclareSizeDecrease(maxAllocs - minAllocs)

	if allocs := thread.Allocations(); allocs != minAllocs {
		t.Errorf("Incorrect number of allocations: expected %d but got %d", minAllocs, allocs)
	}
}

func TestBytesAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "bytes",
		gen: func(n uint) (string, env) {
			return `bytes(b)`, env{"b": dummyString(n, 'b')}
		},
		trend: linear(1),
	})
}

func TestDictAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict",
		gen: func(n uint) (string, env) {
			return "dict(**fields)", env{"fields": dummyDict(n)}
		},
		trend: linear(1),
	})
}

func TestEnumerateAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "enumerate",
		gen: func(n uint) (string, env) {
			return "enumerate(e)", env{"e": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestListAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list",
		gen: func(n uint) (string, env) {
			return "list(l)", env{"l": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestReprAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "repr",
		gen: func(n uint) (string, env) {
			return "repr(s)", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	})
}

func TestSetAllocations(t *testing.T) {
	resolve.AllowSet = true
	testAllocations(t, allocationTest{
		name: "set",
		gen: func(n uint) (string, env) {
			return "set(l)", env{"l": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestStrAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "str_from_str",
		gen: func(n uint) (string, env) {
			return "str(s)", env{"s": dummyString(n, 'a')}
		},
		trend: constant(0),
	})
	testAllocations(t, allocationTest{
		name: "str_from_int",
		gen: func(n uint) (string, env) {
			return "str(i)", env{"i": dummyInt(n)}
		},
		trend: linear(1 / math.Log2(10)),
	})
	testAllocations(t, allocationTest{
		name: "str_from_bytes",
		gen: func(n uint) (string, env) {
			return "str(b)", env{"b": dummyBytes(n, 'a')}
		},
		trend: linear(1),
	})
	testAllocations(t, allocationTest{
		name: "str_from_list",
		gen: func(n uint) (string, env) {
			return "str(l)", env{"l": dummyList(n)}
		},
		trend: linear(float64(len(`"a", `))),
	})
}

func TestTupleAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "tuple",
		gen: func(n uint) (string, env) {
			return "tuple(l)", env{"l": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestZipAllocations(t *testing.T) {
	genZipCall := func(m uint) string {
		entries := make([]string, 0, m)
		for i := uint(1); i <= m; i++ {
			entries = append(entries, fmt.Sprintf("l%d", i))
		}
		return fmt.Sprintf("zip(%s)", strings.Join(entries, ", "))
	}
	genZipEnv := func(n, m uint) env {
		env := make(env, m)
		for i := uint(1); i <= m; i++ {
			env[fmt.Sprintf("l%d", i)] = dummyList(n / m)
		}
		return env
	}

	testAllocations(t, allocationTest{
		name: "zip_pair",
		gen: func(n uint) (string, env) {
			return genZipCall(2), genZipEnv(n, 2)
		},
		trend: linear(1.5), // Allocates backing array
	})
	testAllocations(t, allocationTest{
		name: "zip_quint",
		gen: func(n uint) (string, env) {
			return genZipCall(5), genZipEnv(n, 5)
		},
		trend: linear(1.2), // Allocates backing array
	})
	testAllocations(t, allocationTest{
		name: "zip_collating",
		gen: func(n uint) (string, env) {
			return genZipCall(n), genZipEnv(n, n)
		},
		trend:  affine(1, 3),
		nSmall: 10,
		nLarge: 255,
	})
}

func TestDictItemsAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict.items",
		gen: func(n uint) (string, env) {
			return "d.items()", env{"d": dummyDict(n)}
		},
		trend: linear(1),
	})
}

func TestDictKeysAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict.keys",
		gen: func(n uint) (string, env) {
			return "d.keys()", env{"d": dummyDict(n)}
		},
		trend: linear(1),
	})
}

func TestDictValuesAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict.values",
		gen: func(n uint) (string, env) {
			return "d.values()", env{"d": dummyDict(n)}
		},
		trend: linear(1),
	})
}

func TestListAppendAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list.append",
		gen: func(n uint) (string, env) {
			return strings.Repeat("l.append('a')\n", int(n)), env{"l": starlark.NewList(nil)}
		},
		trend: linear(1),
	})
}

func TestListExtendAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list.extend",
		gen: func(n uint) (string, env) {
			return "l1.extend(l2)", env{
				"l1": dummyList(n),
				"l2": dummyList(n),
			}
		},
		trend: linear(1),
	})
}

func TestListInsertAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list.insert",
		gen: func(n uint) (string, env) {
			return strings.Repeat("l.insert(where, what)\n", int(n)), env{
				"l":     starlark.NewList(nil),
				"where": -1,
				"what":  "a",
			}
		},
		trend: linear(1),
	})
}

func TestStringCapitalizeAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.capitalize",
		gen: func(n uint) (string, env) {
			return "s.capitalize()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	})
}

func TestStringFormatAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "s.format (braces)",
		gen: func(n uint) (string, env) {
			return "s.format()", env{"s": strings.Repeat("{{}}", int(n/4))}
		},
		trend: linear(0.5),
	})

	testAllocations(t, allocationTest{
		name: "string.format (strings)",
		gen: func(n uint) (string, env) {
			return "s.format(*l)", env{
				"s": strings.Repeat("{}", int(n/2)),
				"l": dummyList(n / 2),
			}
		},
		trend: linear(0.5),
	})

	testAllocations(t, allocationTest{
		name: "string.format (ints)",
		gen: func(n uint) (string, env) {
			ints := make([]starlark.Value, 0, n/2)
			for i := uint(0); i < n/2; i++ {
				ints = append(ints, starlark.MakeInt(0))
			}
			return "s.format(*l)", env{
				"s": strings.Repeat("{}", int(n/2)),
				"l": ints,
			}
		},
		trend: linear(0.5),
	})
}

func TestStringJoinAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.join",
		gen: func(n uint) (string, env) {
			return "s.join(l)", env{
				"s": ",",
				"l": dummyList(n / 2),
			}
		},
		trend: linear(1),
	})
}

func TestStringLowerAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.lower",
		gen: func(n uint) (string, env) {
			return "s.lower()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	})
}

func TestStringPartitionAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.partition",
		gen: func(n uint) (string, env) {
			return "s.partition('|')", env{
				"s": dummyString(n/2, 's') + "|" + dummyString(n/2-1, 's'),
			}
		},
		trend: linear(1),
	})
}

func TestStringRemoveprefixAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.removeprefix",
		gen: func(n uint) (string, env) {
			return "s.removeprefix(pre)", env{
				"s":   dummyString(n, 's'),
				"pre": dummyString(n/2, 's'),
			}
		},
		trend: linear(1),
	})
}

func TestStringRemovesuffixAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.removesuffix",
		gen: func(n uint) (string, env) {
			return "s.removesuffix(suf)", env{
				"s":   dummyString(n, 's'),
				"suf": dummyString(n/2, 's'),
			}
		},
		trend: linear(1),
	})
}

func TestStringReplaceAllocations(t *testing.T) {
	for _, expansionFac := range []float64{0.5, 1, 2} {
		testAllocations(t, allocationTest{
			name: fmt.Sprintf("string.replace (with expansion factor %.1f)", expansionFac),
			gen: func(n uint) (string, env) {
				return fmt.Sprintf("s.replace('aa', '%s')", strings.Repeat("b", int(expansionFac*2))), env{"s": dummyString(n, 'a')}
			},
			trend: linear(expansionFac),
		})
	}
}

func TestStringStripAllocations(t *testing.T) {
	whitespaceProportion := 0.5
	testAllocations(t, allocationTest{
		name: "string.strip",
		gen: func(n uint) (string, env) {
			s := new(strings.Builder)
			s.WriteString(strings.Repeat(" ", int(float64(n)*whitespaceProportion*0.5)))
			s.WriteString(string(dummyString(uint(float64(n)*(1-whitespaceProportion)), 'a')))
			s.WriteString(strings.Repeat(" ", int(float64(n)*whitespaceProportion*0.5)))
			return "s.strip()", env{"s": s.String()}
		},
		trend: linear(1 - whitespaceProportion),
	})
}

func TestStringSplitAllocations(t *testing.T) {
	for _, sep := range []string{"", " ", "|"} {
		testAllocations(t, allocationTest{
			name: fmt.Sprintf("string.split (with separator='%s')", sep),
			gen: func(n uint) (string, env) {
				passSep := &sep
				if sep == "" {
					passSep = nil
				}
				return "s.split(sep)", env{
					"s":   generateSepString(n, sep),
					"sep": passSep,
				}
			},
			trend: linear(1),
		})
	}
}

func generateSepString(len uint, sep string) string {
	b := new(strings.Builder)
	b.Grow(int(len))
	{
		const CHUNKS = 10
		for i := 0; i < CHUNKS; i++ {
			if i > 0 {
				b.WriteString(sep)
			}
			b.WriteString(dummyString(len/CHUNKS, 'a'))
		}
	}
	return b.String()
}

func TestStringSplitlinesAllocations(t *testing.T) {
	for _, numLines := range []uint{0, 1, 10, 50} {
		testAllocations(t, allocationTest{
			name: "string.splitlines",
			gen: func(n uint) (string, env) {
				return "s.splitlines()", env{"s": dummyLinesString(n, numLines, 'a')}
			},
			trend: linear(1),
		})
	}
}

func TestStringTitleAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.title",
		gen: func(n uint) (string, env) {
			return "s.title()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	})
}

func TestStringUpperAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.upper",
		gen: func(n uint) (string, env) {
			return "s.upper()", env{"s": dummyString(n, 's')}
		},
		trend: linear(1),
	})
}

func TestSetUnionAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "set.union",
		gen: func(n uint) (string, env) {
			return "s.union(t)", env{
				"s": dummySet(n/2, 0),
				"t": dummySet(n/2, n),
			}
		},
		trend: linear(1),
	})
}

func TestStructAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "struct",
		gen: func(n uint) (string, env) {
			return "struct(**fields)", env{
				"fields": dummyDict(n),
				"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
			}
		},
		trend: linear(2),
	})
}

// Tests allocations follow the speficied trend, within a margin of error
func testAllocations(t *testing.T, test allocationTest) {
	thread := new(starlark.Thread)

	// Test init
	test.InitDefaults()

	// Test allocation increase order
	codeSmall, envSmall := test.gen(test.nSmall)
	predeclsSmall := envSmall.ToStarlarkPredecls()
	deltaSmall, err := memoryIncrease(thread, test.name, codeSmall, predeclsSmall)
	if err != nil && !test.IsFalsePositive(err.Error()) {
		t.Errorf("Unexpected error %v", err)
	}
	codeLarge, envLarge := test.gen(test.nLarge)
	predeclsLarge := envLarge.ToStarlarkPredecls()
	deltaLarge, err := memoryIncrease(thread, test.name, codeLarge, predeclsLarge)
	if err != nil && !test.IsFalsePositive(err.Error()) {
		t.Errorf("Unexpected error %v", err)
	}
	ratio := float64(deltaLarge) / float64(deltaSmall)
	expectedRatio := test.trend.allocations(float64(test.nLarge)) / test.trend.allocations(float64(test.nSmall))
	if ratio <= 0.9*expectedRatio || 1.1*expectedRatio <= ratio {
		t.Errorf("memory allocations did not %s: f(%d)=%d, f(%d)=%d, ratio=%.3f, want ~%.0f", test.trend.label, test.nSmall, deltaSmall, test.nLarge, deltaLarge, ratio, expectedRatio)
	}

	// Test allocations are roughly correct
	expectedAllocs := test.trend.allocations(float64(test.nLarge))
	expectedMinAllocs := uintptr(math.Round(0.9 * expectedAllocs))
	expectedMaxAllocs := uintptr(math.Round(1.1 * expectedAllocs))
	if deltaLarge < expectedMinAllocs {
		t.Errorf("Too few allocations, expected ~%.0f but used only %d", expectedAllocs, deltaLarge)
	}
	if expectedMaxAllocs < deltaLarge {
		t.Errorf("Too many allocations, expected ~%.0f but used %d", expectedAllocs, deltaLarge)
	}
}

// Compute allocation delta declared when executing given code
func memoryIncrease(thread *starlark.Thread, name, code string, predeclared starlark.StringDict) (uintptr, error) {
	allocs0 := thread.Allocations()
	_, err := starlark.ExecFile(thread, name, code, predeclared)
	return thread.Allocations() - allocs0, err
}

func dummyInt(len uint) starlark.Int {
	i := starlark.MakeInt(1)
	i = i.Lsh(len - 1)
	return i
}

func dummyString(len uint, char rune) string {
	return strings.Repeat(string(char), int(len))
}

func dummyLinesString(len, lines uint, char rune) string {
	if lines == 0 {
		return strings.Repeat(string(char), int(len))
	}
	return strings.Repeat(strings.Repeat("a", int(len/lines))+"\n", int(lines))
}

func dummyBytes(len uint, char rune) starlark.Bytes {
	return starlark.Bytes(strings.Repeat(string(char), int(len)))
}

func dummyList(len uint) *starlark.List {
	elems := make([]starlark.Value, 0, len)
	for i := uint(0); i < len; i++ {
		elems = append(elems, starlark.String("a"))
	}
	return starlark.NewList(elems)
}

func dummySet(len, start uint) *starlark.Set {
	set := starlark.NewSet(int(len))
	for i := uint(0); i < len; i++ {
		set.Insert(starlark.MakeInt(int(start + i)))
	}
	return set
}

func dummyDict(len uint) *starlark.Dict {
	dict := starlark.NewDict(int(len))
	for i := 1; i <= int(len); i++ {
		s := starlark.String(strconv.Itoa(i))
		dict.SetKey("_"+s, s)
	}
	return dict
}
