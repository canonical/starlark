package starlark_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestUniverseSafeties(t *testing.T) {
	for name, value := range starlark.Universe {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := (*starlark.UniverseSafeties)[name]; !ok {
			t.Errorf("builtin %s has no safety declaration", name)
		} else if actualSafety := builtin.Safety(); actualSafety != safety {
			t.Errorf("builtin %s has incorrect safety: expected %v but got %v", name, safety, actualSafety)
		}
	}

	for name, _ := range *starlark.UniverseSafeties {
		if _, ok := starlark.Universe[name]; !ok {
			t.Errorf("safety declared for non-existent builtin: %s", name)
		}
	}
}

func TestBytesMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "bytes", starlark.BytesMethods, starlark.BytesMethodSafeties)
}

func TestDictMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "dict", starlark.DictMethods, starlark.DictMethodSafeties)
}

func TestListMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "list", starlark.ListMethods, starlark.ListMethodSafeties)
}

func TestStringMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "string", starlark.StringMethods, starlark.StringMethodSafeties)
}

func TestSetMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "set", starlark.SetMethods, starlark.SetMethodSafeties)
}

func testBuiltinSafeties(t *testing.T, recvName string, builtins map[string]*starlark.Builtin, safeties map[string]starlark.Safety) {
	for name, builtin := range builtins {
		if safety, ok := safeties[name]; !ok {
			t.Errorf("builtin %s.%s has no safety declaration", recvName, name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin %s.%s has incorrect safety: expected %v but got %v", name, recvName, safety, actual)
		}
	}

	for name, _ := range safeties {
		if _, ok := builtins[name]; !ok {
			t.Errorf("safety declared for non-existent builtin %s.%s", recvName, name)
		}
	}
}

// testIterable is an iterable with customisable yield behaviour.
type testIterable struct {
	// If positive, maxN sets an upper bound on the number of iterations
	// performed. Otherwise, iteration is unbounded.
	maxN int

	// nth returns a value to be yielded by the nth Next call.
	nth func(thread *starlark.Thread, n int) (starlark.Value, error)
}

var _ starlark.Iterable = &testIterable{}

func (ti *testIterable) Freeze() {}
func (ti *testIterable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", ti.Type())
}
func (ti *testIterable) String() string       { return "testIterable" }
func (ti *testIterable) Truth() starlark.Bool { return ti.maxN != 0 }
func (ti *testIterable) Type() string         { return "testIterable" }
func (ti *testIterable) Iterate() starlark.Iterator {
	return &testIterator{
		maxN: ti.maxN,
		nth:  ti.nth,
	}
}

type testIterator struct {
	n, maxN int
	nth     func(thread *starlark.Thread, n int) (starlark.Value, error)
	thread  *starlark.Thread
	err     error
}

var _ starlark.SafeIterator = &testIterator{}

func (it *testIterator) BindThread(thread *starlark.Thread) { it.thread = thread }
func (it *testIterator) Safety() starlark.Safety            { return starlark.Safe }
func (it *testIterator) Next(p *starlark.Value) bool {
	it.n++
	if it.nth == nil {
		it.err = errors.New("testIterator called with nil nth function")
	}
	if it.err != nil {
		return false
	}

	if it.maxN > 0 && it.n > it.maxN {
		return false
	}
	ret, err := it.nth(it.thread, it.n)
	if err != nil {
		it.err = err
		return false
	}

	*p = ret
	return true
}
func (it *testIterator) Done()      {}
func (it *testIterator) Err() error { return it.err }

// testSequence is a sequence with customisable yield behaviour.
type testSequence struct {
	// maxN sets the upper bound on the number of iterations performed.
	maxN int

	// nth returns a value to be yielded by the nth Next call.
	nth func(thread *starlark.Thread, n int) (starlark.Value, error)
}

var _ starlark.Sequence = &testSequence{}

func (ts *testSequence) Freeze() {}
func (ts *testSequence) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", ts.Type())
}
func (ts *testSequence) String() string       { return "testSequence" }
func (ts *testSequence) Truth() starlark.Bool { return ts.maxN != 0 }
func (ts *testSequence) Type() string         { return "testSequence" }
func (ts *testSequence) Iterate() starlark.Iterator {
	if ts.maxN < 0 {
		panic(fmt.Sprintf("testSequence is unbounded: got upper bound %v", ts.maxN))
	}
	return &testSequenceIterator{
		testIterator{
			maxN: ts.maxN,
			nth:  ts.nth,
		},
	}
}
func (ts *testSequence) Len() int {
	ret := ts.maxN
	if ret < 0 {
		panic(fmt.Sprintf("testSequence is unbounded: got upper bound %v", ret))
	}
	return ret
}

type testSequenceIterator struct {
	testIterator
}

var _ starlark.SafeIterator = &testSequenceIterator{}

func (iter *testSequenceIterator) Next(p *starlark.Value) bool {
	if iter.maxN == 0 {
		return false
	}
	return iter.testIterator.Next(p)
}

func TestAbsAllocs(t *testing.T) {
}

func TestAnyAllocs(t *testing.T) {
}

func TestAllAllocs(t *testing.T) {
	all, ok := starlark.Universe["all"]
	if !ok {
		t.Fatal("no such builtin: all")
	}

	t.Run("result", func(t *testing.T) {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)

		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{&testIterable{
					maxN: 10,
					nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
						return starlark.True, nil
					},
				}}

				result, err := starlark.Call(thread, all, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("iteration", func(t *testing.T) {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)

		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{&testIterable{
				maxN: st.N,
				nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
					ret := starlark.Bytes(make([]byte, 16))
					st.KeepAlive(ret)
					return ret, thread.AddAllocs(starlark.EstimateSize(ret))
				}},
			}

			result, err := starlark.Call(thread, all, args, nil)
			if err != nil {
				t.Errorf("unexpected error: %s", err.Error())
			}
			st.KeepAlive(result)
		})
	})
}

func TestBoolAllocs(t *testing.T) {
}

func TestBytesAllocs(t *testing.T) {
}

func TestChrAllocs(t *testing.T) {
}

func TestDictAllocs(t *testing.T) {
}

func TestDirAllocs(t *testing.T) {
}

func TestEnumerateAllocs(t *testing.T) {
	enumerate, ok := starlark.Universe["enumerate"]
	if !ok {
		t.Fatal("no such builtin: enumerate")
	}

	t.Run("result", func(t *testing.T) {
		tests := []struct {
			name  string
			value starlark.Iterable
		}{{
			name: "iterable",
			value: &testIterable{
				maxN: 10,
				nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
					return starlark.None, nil
				},
			},
		}, {
			name: "sequence",
			value: &testSequence{
				maxN: 10,
				nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
					return starlark.None, nil
				},
			},
		}}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				st := startest.From(t)

				st.RequireSafety(starlark.MemSafe)

				st.RunThread(func(thread *starlark.Thread) {
					for i := 0; i < st.N; i++ {
						args := starlark.Tuple{test.value}

						result, err := starlark.Call(thread, enumerate, args, nil)
						if err != nil {
							st.Error(err)
						}
						st.KeepAlive(result)
					}
				})
			})
		}
	})

	t.Run("iteration", func(t *testing.T) {
		t.Run("iterable", func(t *testing.T) {
			st := startest.From(t)

			st.RequireSafety(starlark.MemSafe)

			st.RunThread(func(thread *starlark.Thread) {
				iter := &testIterable{
					maxN: st.N,
					nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
						return starlark.None, nil
					},
				}

				result, err := starlark.Call(thread, enumerate, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			})
		})

		t.Run("sequence", func(t *testing.T) {
			st := startest.From(t)

			st.RequireSafety(starlark.MemSafe)

			st.RunThread(func(thread *starlark.Thread) {
				iter := &testSequence{
					maxN: st.N,
					nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
						return starlark.None, nil
					},
				}

				result, err := starlark.Call(thread, enumerate, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			})
		})
	})

	t.Run("early-termination", func(t *testing.T) {
		const expected = "exceeded memory allocation limits"
		maxAllocs := uint64(100)

		t.Run("iterable", func(t *testing.T) {
			st := startest.From(t)

			st.RequireSafety(starlark.MemSafe)
			st.SetMaxAllocs(maxAllocs)

			st.RunThread(func(thread *starlark.Thread) {
				thread.SetMaxAllocs(maxAllocs)

				var nReached int
				iter := &testIterable{
					maxN: st.N,
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						nReached = n
						return starlark.None, nil
					},
				}

				result, err := starlark.Call(thread, enumerate, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				if nReached > 1 && iter.maxN != 1 {
					st.Errorf("iteration was not terminated early enough")
				}

				st.KeepAlive(result)
			})
		})

		t.Run("sequence", func(t *testing.T) {
			st := startest.From(t)

			st.RequireSafety(starlark.MemSafe)
			st.SetMaxAllocs(maxAllocs)

			st.RunThread(func(thread *starlark.Thread) {
				thread.SetMaxAllocs(maxAllocs)

				var nReached int
				iter := &testSequence{
					maxN: st.N,
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						nReached = n
						return starlark.None, nil
					},
				}

				result, err := starlark.Call(thread, enumerate, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				if nReached > 0 && iter.maxN > 1 {
					st.Errorf("iteration was not terminated early enough: terminated after %d/%d Next calls", nReached+1, iter.Len())
				}

				st.KeepAlive(result)
			})
		})
	})
}

func TestFailAllocs(t *testing.T) {
}

func TestFloatAllocs(t *testing.T) {
}

func TestGetattrAllocs(t *testing.T) {
}

func TestHasattrAllocs(t *testing.T) {
}

func TestHashAllocs(t *testing.T) {
}

func TestIntAllocs(t *testing.T) {
	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			inputString := starlark.String("deadbeef")
			args := []starlark.Value{inputString, starlark.MakeInt(16)}

			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, starlark.Universe["int"], args, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			inputString := starlark.String(strings.Repeat("deadbeef", st.N))
			args := []starlark.Value{inputString, starlark.MakeInt(16)}

			result, err := starlark.Call(th, starlark.Universe["int"], args, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		})
	})
}

func TestLenAllocs(t *testing.T) {
}

func TestListAllocs(t *testing.T) {
}

func TestMaxAllocs(t *testing.T) {
}

func TestMinAllocs(t *testing.T) {
}

func TestOrdAllocs(t *testing.T) {
}

func TestPrintAllocs(t *testing.T) {
}

func TestRangeAllocs(t *testing.T) {
}

func TestReprAllocs(t *testing.T) {
}

func TestReversedAllocs(t *testing.T) {
}

func TestSetAllocs(t *testing.T) {
}

func TestSortedAllocs(t *testing.T) {
}

func TestStrAllocs(t *testing.T) {
}

func TestTupleAllocs(t *testing.T) {
}

func TestTypeAllocs(t *testing.T) {
}

func TestZipAllocs(t *testing.T) {
}

func TestBytesElemsAllocs(t *testing.T) {
}

func TestDictClearAllocs(t *testing.T) {
}

func TestDictGetAllocs(t *testing.T) {
}

func TestDictItemsAllocs(t *testing.T) {
}

func TestDictKeysAllocs(t *testing.T) {
}

func TestDictPopAllocs(t *testing.T) {
}

func TestDictPopitemAllocs(t *testing.T) {
}

func TestDictSetdefaultAllocs(t *testing.T) {
}

func TestDictUpdateAllocs(t *testing.T) {
}

func TestDictValuesAllocs(t *testing.T) {
}

func TestListAppendAllocs(t *testing.T) {
}

func TestListClearAllocs(t *testing.T) {
}

func TestListExtendAllocs(t *testing.T) {
}

func TestListIndexAllocs(t *testing.T) {
}

func TestListInsertAllocs(t *testing.T) {
}

func TestListPopAllocs(t *testing.T) {
}

func TestListRemoveAllocs(t *testing.T) {
}

func TestStringCapitalizeAllocs(t *testing.T) {
}

func TestStringCodepoint_ordsAllocs(t *testing.T) {
}

func TestStringCodepointsAllocs(t *testing.T) {
}

func TestStringCountAllocs(t *testing.T) {
}

func TestStringElem_ordsAllocs(t *testing.T) {
}

func TestStringElemsAllocs(t *testing.T) {
}

func TestStringEndswithAllocs(t *testing.T) {
}

func TestStringFindAllocs(t *testing.T) {
}

func TestStringFormatAllocs(t *testing.T) {
}

func TestStringIndexAllocs(t *testing.T) {
}

func TestStringIsalnumAllocs(t *testing.T) {
}

func TestStringIsalphaAllocs(t *testing.T) {
}

func TestStringIsdigitAllocs(t *testing.T) {
}

func TestStringIslowerAllocs(t *testing.T) {
}

func TestStringIsspaceAllocs(t *testing.T) {
}

func TestStringIstitleAllocs(t *testing.T) {
}

func TestStringIsupperAllocs(t *testing.T) {
}

func TestStringJoinAllocs(t *testing.T) {
}

func TestStringLowerAllocs(t *testing.T) {
}

func TestStringLstripAllocs(t *testing.T) {
}

func TestStringPartitionAllocs(t *testing.T) {
}

func TestStringRemoveprefixAllocs(t *testing.T) {
}

func TestStringRemovesuffixAllocs(t *testing.T) {
}

func TestStringReplaceAllocs(t *testing.T) {
}

func TestStringRfindAllocs(t *testing.T) {
}

func TestStringRindexAllocs(t *testing.T) {
}

func TestStringRpartitionAllocs(t *testing.T) {
}

func TestStringRsplitAllocs(t *testing.T) {
}

func TestStringRstripAllocs(t *testing.T) {
}

func TestStringSplitAllocs(t *testing.T) {
}

func TestStringSplitlinesAllocs(t *testing.T) {
}

func TestStringStartswithAllocs(t *testing.T) {
}

func TestStringStripAllocs(t *testing.T) {
}

func TestStringTitleAllocs(t *testing.T) {
}

func TestStringUpperAllocs(t *testing.T) {
}

func TestSetUnionAllocs(t *testing.T) {
}

type repeatIterable struct {
	n     int
	value starlark.Value
}

func (r *repeatIterable) Freeze()               {}
func (r *repeatIterable) Hash() (uint32, error) { return 0, fmt.Errorf("invalid") }
func (r *repeatIterable) Iterate() starlark.Iterator {
	value := r.value
	if value == nil {
		value = starlark.None
	}
	return &repeatIterator{
		n:     r.n,
		value: value,
	}
}
func (r *repeatIterable) String() string       { return "repeat" }
func (r *repeatIterable) Truth() starlark.Bool { return r.n > 0 }
func (r *repeatIterable) Type() string         { return "repeat" }

type repeatIterator struct {
	n     int
	value starlark.Value
}

func (it *repeatIterator) Done()                              {}
func (it *repeatIterator) Err() error                         { return nil }
func (it *repeatIterator) NextAllocs() int64                  { return 0 }
func (it *repeatIterator) BindThread(thread *starlark.Thread) {}

func (it *repeatIterator) Safety() starlark.Safety {
	return starlark.MemSafe
}

func (it *repeatIterator) Next(p *starlark.Value) bool {
	if it.n <= 0 {
		*p = nil
		return false
	}
	it.n--
	*p = it.value
	return true
}

type allocatingIterable struct {
	size int
}

func (si *allocatingIterable) Freeze()               {}
func (si *allocatingIterable) Hash() (uint32, error) { return 0, fmt.Errorf("invalid") }
func (si *allocatingIterable) String() string        { return "stringifyIterable" }
func (si *allocatingIterable) Truth() starlark.Bool  { return starlark.False }
func (si *allocatingIterable) Type() string          { return "stringifyIterable" }

func (si *allocatingIterable) Iterate() starlark.Iterator {
	return &allocatingIterator{size: si.size}
}

type allocatingIterator struct {
	size   int
	thread *starlark.Thread
	err    error
}

var _ starlark.SafeIterator = &allocatingIterator{}

func (it *allocatingIterator) Done()                              {}
func (it *allocatingIterator) BindThread(thread *starlark.Thread) { it.thread = thread }
func (it *allocatingIterator) Err() error                         { return it.err }
func (it *allocatingIterator) Safety() starlark.Safety            { return starlark.MemSafe }

func (it *allocatingIterator) Next(p *starlark.Value) bool {
	list := starlark.NewList(make([]starlark.Value, 0, it.size))

	if it.thread != nil {
		if err := it.thread.AddAllocs(starlark.EstimateSize(list)); err != nil {
			it.err = err
			return false
		}
	}

	*p = list
	return true
}

func TestSafeIterateAllocs(t *testing.T) {
	t.Run("non-allocating", func(t *testing.T) {
		st := startest.From(t)

		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			nonAllocating := &repeatIterable{st.N, starlark.True}
			it, err := starlark.SafeIterate(thread, nonAllocating)
			if err != nil {
				t.Fatal(err)
			}

			defer it.Done()

			for i := 0; i < st.N; i++ {
				var value starlark.Value
				if !it.Next(&value) {
					st.Errorf("non-terminating iterator stuck at %d", st.N)
					return
				}

				st.KeepAlive(value)
			}
		})
	})

	t.Run("allocating", func(t *testing.T) {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			allocating := &allocatingIterable{16}
			it, err := starlark.SafeIterate(thread, allocating)
			if err != nil {
				t.Fatal(err)
			}

			defer it.Done()

			for i := 0; i < st.N; i++ {
				var value starlark.Value
				if !it.Next(&value) {
					st.Errorf("non-terminating iterator stuck at %d", st.N)
					return
				}

				st.KeepAlive(value)
			}
		})
	})
}
