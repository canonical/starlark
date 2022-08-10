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

func TestAbsAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "abs (positive)",
		gen: func(n uint) (string, env) {
			return `abs(n)`, env{"n": dummyInt(n)}
		},
		trend: constant(0),
	})
	testAllocations(t, allocationTest{
		name: "abs (negative float)",
		gen: func(n uint) (string, env) {
			return `abs(f)`, env{"f": -float64(n)}
		},
		trend: constant(1),
	})
	testAllocations(t, allocationTest{
		name: "abs (negative int)",
		gen: func(n uint) (string, env) {
			return `abs(n)`, env{"n": starlark.MakeInt(0).Sub(dummyInt(n))}
		},
		trend: linear(float64(dummyInt(100000).Size()) / 100000),
	})
}

func TestAllAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "all",
		gen: func(n uint) (string, env) {
			return `all(l)`, env{"l": dummyList(n)}
		},
		trend: constant(0),
	})
}

func TestAnyAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "any",
		gen: func(n uint) (string, env) {
			return `any(l)`, env{"l": dummyList(n)}
		},
		trend: constant(1),
	})
}

func TestBoolAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "bool",
		gen: func(n uint) (string, env) {
			return `bool(v)`, env{"v": dummyString(n, 'a')}
		},
		trend: constant(1),
	})
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

func TestChrAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "chr",
		gen: func(_ uint) (string, env) {
			return `chr(i)`, env{"i": 65}
		},
		trend: constant(2),
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

// TODO(kcza): test dir allocations. Some implementations of HasAttrs may
// return the same []string with each invocation, others may generate it. The
// number of allocations must be specified by the implementor.

func TestEnumerateAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "enumerate",
		gen: func(n uint) (string, env) {
			return "enumerate(e)", env{"e": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestFailAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "fail",
		gen: func(n uint) (string, env) {
			return "fail(s)", env{"s": dummyString(n, 'a')}
		},
		trend:                     linear(1),
		falsePositiveCancellation: regexp.MustCompile("^fail: a+$"),
	})
	testAllocations(t, allocationTest{
		name: "fail_with_sep",
		gen: func(n uint) (string, env) {
			return "fail(sep=sep, *l)", env{
				"l":   dummyList(n),
				"sep": 'b',
			}
		},
		trend:                     linear(2),
		falsePositiveCancellation: regexp.MustCompile("^fail: (ab)*a$"),
	})
}

func TestFloatAllocations(t *testing.T) {
	vals := []interface{}{
		1000,
		1000.0,
		"100000",
		"infinity",
		"-infinity",
		"inf",
		"-inf",
		"1.748302748932047839274389274374892730478234",
	}
	for _, v := range vals {
		testAllocations(t, allocationTest{
			name: "float",
			gen: func(n uint) (string, env) {
				return `float(v)`, env{"v": v}
			},
			trend: constant(1),
		})
	}
}

// TODO(kcza): test getattr allocations. Some implementations of
// starlark.HasAttrs may return the same []string with each invocation, others
// may generate it. The number of allocations must be specified by the
// implementor.

// TODO(kcza): test setattr allocations. Some implementations of
// starlark.HasSetField may return the same []string with each invocation,
// others may generate it. The number of allocations must be specified by the
// implementor.

func TestHashAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "hash (string)",
		gen: func(n uint) (string, env) {
			return `hash(val)`, env{"val": dummyString(n, 'a')}
		},
		trend: constant(1),
	})
	testAllocations(t, allocationTest{
		name: "hash (bytes)",
		gen: func(n uint) (string, env) {
			return `hash(val)`, env{"val": dummyBytes(n, 'a')}
		},
		trend: constant(1),
	})
}

func TestLenAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "len",
		gen: func(n uint) (string, env) {
			return `len(l)`, env{"l": dummyList(n)}
		},
		trend: constant(1),
	})
}

func TestListAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list",
		gen: func(n uint) (string, env) {
			return `list(l)`, env{"l": dummyList(n)}
		},
		trend: linear(1),
	})
}

func TestMaxAllocations(t *testing.T) {
	testMinMaxAllocations(t, "max")
}

func TestMinAllocations(t *testing.T) {
	testMinMaxAllocations(t, "min")
}

func testMinMaxAllocations(t *testing.T, funcName string) {
	testAllocations(t, allocationTest{
		name: funcName,
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`%s(l)`, funcName), env{"l": dummyList(n)}
		},
		trend: constant(0),
	})
}

func TestOrdAllocations(t *testing.T) {
	for _, v := range []interface{}{"q", starlark.Bytes(string("v"))} {
		testAllocations(t, allocationTest{
			name: "ord",
			gen: func(n uint) (string, env) {
				return `ord(v)`, env{"v": v}
			},
			trend: constant(1),
		})
	}
}

func TestRangeAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "range",
		gen: func(n uint) (string, env) {
			return `range(n)`, env{"n": n}
		},
		trend: constant(1),
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

func TestReversedAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "reversed",
		gen: func(n uint) (string, env) {
			return `reversed(l)`, env{"l": dummyList(n)}
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

func TestSortedAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "sorted",
		gen: func(n uint) (string, env) {
			return `sorted(l)`, env{
				"l": dummyList(n),
			}
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

// TODO(kcza): test type allocations. Some implementations of starlark.Value
// may retur n a different string depending on the data it represents. For
// example, a matrix type may wish to declare its type as `Matrix[n,m]`, for
// example.

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

func TestBytesElemsAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "bytes.elems",
		gen: func(n uint) (string, env) {
			return `b.elems()`, env{"b": dummyBytes(n, 'a')}
		},
		trend: constant(1),
	})
}

func TestDictGetAllocations(t *testing.T) {
	for _, testKeyPresent := range []bool{false, true} {
		testAllocations(t, allocationTest{
			name: "dict.get",
			gen: func(n uint) (string, env) {
				d := starlark.NewDict(1)
				if testKeyPresent {
					d.SetKey(starlark.String("k"), starlark.String(dummyString(n, 'v')))
				}
				return `d.get(k)`, env{
					"d": d,
					"k": "k",
				}
			},
			trend: constant(0),
		})
	}
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

func TestDictSetDefaultAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict.setdefault (default absent, value absent)",
		gen: func(n uint) (string, env) {
			return `d.setdefault(k)`, env{
				"d": starlark.NewDict(0),
				"k": dummyInt(n),
			}
		},
		trend: constant(1),
	})

	testAllocations(t, allocationTest{
		name: "dict.setdefault (default absent, value present)",
		gen: func(n uint) (string, env) {
			d := starlark.NewDict(1)
			k := starlark.String("k")
			v := dummyInt(n)
			d.SetKey(k, v)
			return `d.setdefault(k)`, env{
				"d": d,
				"k": k,
			}
		},
		trend: constant(0),
	})

	testAllocations(t, allocationTest{
		name: "dict.setdefault (default present, value absent)",
		gen: func(n uint) (string, env) {
			return `d.setdefault(k, v)`, env{
				"d": starlark.NewDict(0),
				"k": "k",
				"v": dummyInt(n),
			}
		},
		trend: constant(1),
	})

	testAllocations(t, allocationTest{
		name: "dict.setdefault (default present, value present)",
		gen: func(n uint) (string, env) {
			d := starlark.NewDict(1)
			k := starlark.String("k")
			v := dummyInt(n)
			d.SetKey(k, v)
			return `d.setdefault(k, v)`, env{
				"d": d,
				"k": k,
				"v": v,
			}
		},
		trend: constant(0),
	})

}

func TestDictUpdateAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "dict.update (from list, no overlap)",
		gen: func(n uint) (string, env) {
			l := make([]starlark.Value, 0, n)
			for i := 0; i < int(n); i++ {
				t := make(starlark.Tuple, 0, 2)
				t = append(t, starlark.MakeInt(i), starlark.String("a"))
				l = append(l, t)
			}
			return `d.update(l)`, env{
				"d": starlark.NewDict(int(n)),
				"l": l,
			}
		},
		trend: linear(1),
	})

	testAllocations(t, allocationTest{
		name: "dict.update (kwargs, no overlap)",
		gen: func(n uint) (string, env) {
			kvPairs := make([]string, 0, n)
			for i := uint(0); i < n; i++ {
				kvPairs = append(kvPairs, fmt.Sprintf("_%d='%s'", i, "a"))
			}
			return fmt.Sprintf(`d.update(%s)`, strings.Join(kvPairs, ",")), env{
				"d": starlark.NewDict(int(n)),
			}
		},
		trend:  linear(1),
		nSmall: 25,
		nLarge: 255,
	})

	testAllocations(t, allocationTest{
		name: "dict.update (from dict, no overlap)",
		gen: func(n uint) (string, env) {
			d2 := starlark.NewDict(int(n))
			for i := 0; i < int(n); i++ {
				k := starlark.String(strconv.Itoa(i))
				v := starlark.String(strconv.Itoa(i))
				d2.SetKey(k, v)
			}
			return `d.update(d2)`, env{
				"d":  starlark.NewDict(int(n)),
				"d2": d2,
			}
		},
		trend: linear(1),
	})

	testAllocations(t, allocationTest{
		name: "dict.update (from list, with overlap)",
		gen: func(n uint) (string, env) {
			d := starlark.NewDict(int(n))
			l := make([]starlark.Value, 0, n)
			for i := 0; i < int(n); i++ {
				k := starlark.String(fmt.Sprintf("_%d", i))
				v := starlark.MakeInt(i)
				// Create overlap of 50%
				if i < int(n)/2 {
					d.SetKey(k, v)
				}
				t := append(make(starlark.Tuple, 0, 2), k, v)
				l = append(l, t)
			}
			return `d.update(l)`, env{
				"d": d,
				"l": l,
			}
		},
		trend: linear(0.5),
	})

	testAllocations(t, allocationTest{
		name: "dict.update (from kwargs, with overlap)",
		gen: func(n uint) (string, env) {
			d := starlark.NewDict(int(1.5 * float64(n)))
			d2 := starlark.NewDict(int(n))
			for i := 0; i < int(n); i++ {
				s := starlark.String(strconv.Itoa(i))
				d.SetKey(s, s)
				s2 := starlark.String(strconv.Itoa(int(float64(i) * 2)))
				d2.SetKey(s2, s2)
			}
			return `d.update(**d2)`, env{
				"d":  d,
				"d2": d2,
			}
		},
		trend: linear(0.5),
	})

	testAllocations(t, allocationTest{
		name: "dict.update (from dict, with overlap)",
		gen: func(n uint) (string, env) {
			d := starlark.NewDict(int(n))
			d2 := starlark.NewDict(int(n))
			for i := 0; i < int(n); i++ {
				k := starlark.String(strconv.Itoa(i))
				v := starlark.MakeInt(i)
				// Create overlap of 50%
				if i < int(n)/2 {
					d.SetKey(k, v)
				}
				d2.SetKey(k, v)
			}
			return `d.update(d2)`, env{
				"d":  d,
				"d2": d2,
			}
		},
		trend: linear(0.5),
	})

	testAllocations(t, allocationTest{
		name: "dict.update (from list and dict, with overlap)",
		gen: func(n uint) (string, env) {
			// Generate a dictionary d, which overlaps 50% with a dictionary d2
			// and dict(l), where d2 and dict(l) are disjoint, update d with l
			// and d2, causing d to double in size.

			d := starlark.NewDict(int(1.75 * float64(n)))
			d2 := starlark.NewDict(int(n))
			l := make([]starlark.Value, 0, n)
			for i := 0; i < int(n); i++ {
				s := starlark.String(strconv.Itoa(i))
				d.SetKey(s, s)

				s2 := starlark.String(strconv.Itoa(i * 2))
				d2.SetKey(s2, s2)

				s3 := starlark.String(strconv.Itoa(i*2 + 1))
				l = append(l, starlark.Tuple([]starlark.Value{s3, s3}))
			}
			return `d.update(l, **d2)`, env{
				"d":  d,
				"l":  l,
				"d2": d2,
			}
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

func TestListIndexAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "list.index (value absent)",
		gen: func(n uint) (string, env) {
			return `l.index(s)`, env{
				"l": make([]starlark.Value, 0),
				"s": dummyString(n, 'a'),
			}
		},
		trend:                     constant(0),
		falsePositiveCancellation: regexp.MustCompile("value not in list"),
	})

	testAllocations(t, allocationTest{
		name: "list.index (value present)",
		gen: func(n uint) (string, env) {
			s := starlark.String(dummyString(n, 'a'))
			l := make([]starlark.Value, 0, 1)
			l = append(l, s)
			return `l.index(s)`, env{
				"l": l,
				"s": s,
			}
		},
		trend: constant(1),
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

func TestStringCodepointordsAllocations(t *testing.T) {
	testStringIterableMethod(t, "codepoint_ords")
}

func TestStringCodepointsAllocations(t *testing.T) {
	testStringIterableMethod(t, "codepoints")
}

func TestStringCountAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.count",
		gen: func(n uint) (string, env) {
			return `s.count(t)`, env{
				"s": dummyString(n, 'a'),
				"t": 'a',
			}
		},
		// Technically this trend is logarithmic, but it will only surpass 1
		// for strings with more characters than there are atoms in the
		// universe. For our mortal purposes, it is a constant 1.
		trend: constant(1),
	})
}

func TestStringElemordsAllocations(t *testing.T) {
	testStringIterableMethod(t, "elem_ords")
}

func TestStringElemsAllocations(t *testing.T) {
	testStringIterableMethod(t, "elems")
}

func testStringIterableMethod(t *testing.T, name string) {
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s()`, name), env{"s": dummyString(n, 'a')}
		},
		trend: constant(1),
	})
}

func TestStringEndswithAllocations(t *testing.T) {
	testStringStartsEndsWithAllocations(t, "endswith")
}

func TestStringFindAllocations(t *testing.T) {
	testStringFindMethod(t, "find", true)
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

func TestStringIndexAllocations(t *testing.T) {
	testStringFindMethod(t, "index", false)
}

func TestStringIsalnumAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "isalnum", 'a', '.')
}

func TestStringIsalphaAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "isalpha", 'a', '1')
}

func TestStringIsdigitAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "isdigit", '1', 'a')
}

func TestStringIslowerAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "islower", 'a', 'A')
}

func TestStringIsspaceAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "isspace", ' ', '.')
}

func TestStringIstitleAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.istitle (false)",
		gen: func(n uint) (string, env) {
			return `s.istitle()`, env{
				"s": dummyString(n, 'a'),
			}
		},
		trend: constant(1),
	})
	testAllocations(t, allocationTest{
		name: "string.istitle (true)",
		gen: func(n uint) (string, env) {
			return `s.istitle()`, env{"s": strings.Repeat("Aa", int(n))}
		},
		trend: constant(1),
	})
}

func TestStringIsupperAllocations(t *testing.T) {
	testStringIsPatternAllocations(t, "isupper", 'A', 'a')
}

func testStringIsPatternAllocations(t *testing.T, name string, trueRune, falseRune rune) {
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (false)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s()`, name), env{"s": dummyString(n, falseRune)}
		},
		trend: constant(1),
	})

	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (true)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s()`, name), env{"s": dummyString(n, trueRune)}
		},
		trend: constant(1),
	})
}

func TestStringLstripAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.lstrip",
		gen: func(n uint) (string, env) {
			s := new(strings.Builder)
			s.Grow(int(n))
			s.WriteString(strings.Repeat(" ", int(n/2)))
			s.WriteString(strings.Repeat("a", int(n/2)))
			return `s.lstrip()`, env{"s": s.String()}
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
	testStringPartitionMethodAllocations(t, "partition")
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

func TestStringRfindAllocations(t *testing.T) {
	testStringFindMethod(t, "rfind", true)
}

func TestStringRindexAllocations(t *testing.T) {
	testStringFindMethod(t, "rindex", false)
}

func testStringFindMethod(t *testing.T, name string, allocOnAbsent bool) {
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (present)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s(t)`, name), env{
				"s": dummyString(n, 'a'),
				"t": dummyString(n/2, 'a'),
			}
		},
		trend: constant(1),
	})

	absentAllocs := 0.0
	if allocOnAbsent {
		absentAllocs = 1
	}
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (absent)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s(t)`, name), env{
				"s": dummyString(n, 'a'),
				"t": dummyString(n/2, 'b'),
			}
		},
		trend:                     constant(absentAllocs),
		falsePositiveCancellation: regexp.MustCompile("index: substring not found"),
	})
}

func TestStringRpartitionAllocations(t *testing.T) {
	testStringPartitionMethodAllocations(t, "rpartition")
}

func testStringPartitionMethodAllocations(t *testing.T, name string) {
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf("s.%s('|')", name), env{
				"s": dummyString(n/2, 's') + "|" + dummyString(n/2-1, 's'),
			}
		},
		trend: linear(1),
	})
}

func TestStringRsplitAllocations(t *testing.T) {
	testStringSplitAllocations(t, "rsplit")
}

func TestStringRstripAllocations(t *testing.T) {
	testAllocations(t, allocationTest{
		name: "string.rstrip",
		gen: func(n uint) (string, env) {
			s := new(strings.Builder)
			s.Grow(int(n))
			s.WriteString(strings.Repeat("a", int(n/2)))
			s.WriteString(strings.Repeat(" ", int(n/2)))
			return `s.rstrip()`, env{"s": s.String()}
		},
		trend: linear(0.5),
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
	testStringSplitAllocations(t, "split")
}

func testStringSplitAllocations(t *testing.T, name string) {
	for _, sep := range []string{"", " ", "|"} {
		testAllocations(t, allocationTest{
			name: fmt.Sprintf("string.%s (with separator='%s')", name, sep),
			gen: func(n uint) (string, env) {
				passSep := &sep
				if sep == "" {
					passSep = nil
				}
				return fmt.Sprintf(`s.%s(sep)`, name), env{
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

func TestStringStartswithAllocations(t *testing.T) {
	testStringStartsEndsWithAllocations(t, "startswith")
}

func testStringStartsEndsWithAllocations(t *testing.T, name string) {
	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (false)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s(t)`, name), env{
				"s": dummyString(n, 'a'),
				"t": dummyString(n/2, 'b'),
			}
		},
		trend: constant(1),
	})

	testAllocations(t, allocationTest{
		name: fmt.Sprintf("string.%s (true)", name),
		gen: func(n uint) (string, env) {
			return fmt.Sprintf(`s.%s(t)`, name), env{
				"s": dummyString(n, 'a'),
				"t": dummyString(n/2, 'a'),
			}
		},
		trend: constant(1),
	})
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
		t.Errorf("%s: unexpected error %v", test.name, err)
	}
	codeLarge, envLarge := test.gen(test.nLarge)
	predeclsLarge := envLarge.ToStarlarkPredecls()
	deltaLarge, err := memoryIncrease(thread, test.name, codeLarge, predeclsLarge)
	if err != nil && !test.IsFalsePositive(err.Error()) {
		t.Errorf("%s: unexpected error %v", test.name, err)
	}
	ratio := float64(deltaLarge) / float64(deltaSmall)
	expectedRatio := test.trend.allocations(float64(test.nLarge)) / test.trend.allocations(float64(test.nSmall))
	if ratio <= 0.9*expectedRatio || 1.1*expectedRatio <= ratio {
		t.Errorf("%s: memory allocations did not %s: f(%d)=%d, f(%d)=%d, ratio=%.3f, want ~%.0f", test.name, test.trend.label, test.nSmall, deltaSmall, test.nLarge, deltaLarge, ratio, expectedRatio)
	}

	// Test allocations are roughly correct
	expectedAllocs := test.trend.allocations(float64(test.nLarge))
	expectedMinAllocs := uintptr(math.Round(0.9 * expectedAllocs))
	expectedMaxAllocs := uintptr(math.Round(1.1 * expectedAllocs))
	if deltaLarge < expectedMinAllocs {
		t.Errorf("%s: too few allocations, expected ~%.0f but used only %d", test.name, expectedAllocs, deltaLarge)
	}
	if expectedMaxAllocs < deltaLarge {
		t.Errorf("%s: too many allocations, expected ~%.0f but used %d", test.name, expectedAllocs, deltaLarge)
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
