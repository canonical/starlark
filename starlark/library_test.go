package starlark_test

import (
	"errors"
	"fmt"
	"math"
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
func (it *testIterator) Safety() starlark.Safety {
	if it.thread == nil {
		return starlark.NotSafe
	}
	return starlark.Safe
}
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
	bool_ := starlark.Universe["bool"]
	values := []starlark.Value{
		starlark.None,
		starlark.True,
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 40),
		starlark.String("deadbeef"),
		starlark.NewSet(10),
		starlark.NewDict(10),
		starlark.NewList(nil),
		starlark.Float(0.5),
	}

	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	for _, value := range values {
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value, err := starlark.Call(thread, bool_, starlark.Tuple{value}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(value)
			}
		})
	}
}

func TestBytesAllocs(t *testing.T) {
}

func TestChrAllocs(t *testing.T) {
	chr := starlark.Universe["chr"]

	st := startest.From(t)

	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(32)

	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			args := starlark.Tuple{starlark.MakeInt(97)}
			result, err := starlark.Call(thread, chr, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
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

	t.Run("small-result", func(t *testing.T) {
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

	t.Run("large-result", func(t *testing.T) {
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
		maxAllocs := uint64(40)

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

	listLoopContent := []starlark.Value{nil}
	var listLoop starlark.Value = starlark.NewList(listLoopContent)
	listLoopContent[0] = listLoop

	dictLoop := starlark.NewDict(1)
	var dictLoopValue starlark.Value = dictLoop
	dictLoop.SetKey(starlark.MakeInt(0x1CEB00DA), dictLoopValue)

	args := starlark.Tuple{
		starlark.True,
		listLoop,
		dictLoop,
		starlark.Float(math.Phi),
		starlark.NewSet(1),
		starlark.String(`"'{}🌋`),
	}

	st := startest.From(t)
	fn := starlark.Universe["fail"]
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			res, err := starlark.Call(thread, fn, args, nil)
			if err == nil {
				st.Errorf("fail returned success %v", res)
			}
			st.KeepAlive(err.Error())
			thread.AddAllocs(starlark.StringTypeOverhead)
		}
	})
}

func TestFloatAllocs(t *testing.T) {
	float := starlark.Universe["float"]

	values := []starlark.Value{
		starlark.True,
		starlark.MakeInt(1),
		starlark.MakeInt64(1 << 32),
		starlark.Float(1 << 32),
		starlark.String("2147483648"),
		starlark.String("18446744073709551616"),
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	for _, value := range values {
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, float, starlark.Tuple{value}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	}
}

func TestGetattrAllocs(t *testing.T) {
}

func TestHasattrAllocs(t *testing.T) {
}

func TestHashAllocs(t *testing.T) {
	hash := starlark.Universe["hash"]

	t.Run("input=string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.String("foo")}
				result, err := starlark.Call(thread, hash, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("input=bytes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.String("bar")}
				result, err := starlark.Call(thread, hash, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
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
	listLoopContent := []starlark.Value{nil}
	var listLoop starlark.Value = starlark.NewList(listLoopContent)
	listLoopContent[0] = listLoop

	dictLoop := starlark.NewDict(1)
	var dictLoopValue starlark.Value = dictLoop
	dictLoop.SetKey(starlark.MakeInt(0x1CEB00DA), dictLoopValue)

	args := starlark.Tuple{
		starlark.True,
		listLoop,
		dictLoop,
		starlark.Float(math.Phi),
		starlark.NewSet(1),
		starlark.String(`"'{}🌋`),
	}

	st := startest.From(t)
	fn := starlark.Universe["print"]
	printFn := func(thread *starlark.Thread, msg string) {
		if err := thread.AddAllocs(starlark.StringTypeOverhead); err != nil {
			st.Error(err)
		}
		st.KeepAlive(msg)
	}
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		thread.Print = printFn
		for i := 0; i < st.N; i++ {
			res, err := starlark.Call(thread, fn, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(res)
		}
	})
}

func TestRangeAllocs(t *testing.T) {
	range_ := starlark.Universe["range"]

	t.Run("non-enumerating", func(t *testing.T) {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(10000), starlark.MakeInt(1)}
				result, err := starlark.Call(thread, range_, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("enumerating", func(t *testing.T) {
		st := startest.From(t)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(st.N), starlark.MakeInt(1)}
			result, err := starlark.Call(thread, range_, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)

			iter, err := starlark.SafeIterate(thread, result)
			if err != nil {
				st.Error(err)
			}
			defer iter.Done()

			var value starlark.Value
			for iter.Next(&value) {
				st.KeepAlive(value)
			}
			if err := iter.Err(); err != nil {
				st.Error(err)
			}
		})
	})
}

func TestReprAllocs(t *testing.T) {
	listLoopContent := []starlark.Value{nil}
	var listLoop starlark.Value = starlark.NewList(listLoopContent)
	listLoopContent[0] = listLoop

	dictLoop := starlark.NewDict(1)
	var dictLoopValue starlark.Value = dictLoop
	dictLoop.SetKey(starlark.MakeInt(0x1CEB00DA), dictLoopValue)

	args := starlark.Tuple{
		starlark.True,
		listLoop,
		dictLoop,
		starlark.Float(math.Phi),
		starlark.NewSet(1),
		starlark.String(`"'{}🌋`),
	}

	st := startest.From(t)
	fn := starlark.Universe["repr"]
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			res, err := starlark.Call(thread, fn, starlark.Tuple{args}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(res)
		}
	})
}

func TestReversedAllocs(t *testing.T) {
}

func TestSetAllocs(t *testing.T) {
}

func TestSortedAllocs(t *testing.T) {
}

func TestStrAllocs(t *testing.T) {
	listLoopContent := []starlark.Value{nil}
	var listLoop starlark.Value = starlark.NewList(listLoopContent)
	listLoopContent[0] = listLoop

	dictLoop := starlark.NewDict(1)
	var dictLoopValue starlark.Value = dictLoop
	dictLoop.SetKey(starlark.MakeInt(0x1CEB00DA), dictLoopValue)

	fn := starlark.Universe["str"]
	args := starlark.Tuple{
		starlark.True,
		listLoop,
		dictLoop,
		starlark.Float(math.Phi),
		starlark.NewSet(1),
		starlark.String(`"'{}🌋`),
	}

	t.Run("no-op", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				res, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("any string `\"'")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(res)
			}
		})
	})

	t.Run("conversion", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				res, err := starlark.Call(thread, fn, starlark.Tuple{args}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(res)
			}
		})
	})
}

func TestTupleAllocs(t *testing.T) {
}

func TestTypeAllocs(t *testing.T) {
}

func TestZipAllocs(t *testing.T) {
	zip := starlark.Universe["zip"]

	t.Run("populated", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			tuple := make(starlark.Tuple, st.N)

			result, err := starlark.Call(thread, zip, starlark.Tuple{tuple, tuple}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		})
	})

	nth := func(*starlark.Thread, int) (starlark.Value, error) {
		return starlark.True, nil
	}

	t.Run("lazy-sequence", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			sequence := &testSequence{st.N, nth}

			result, err := starlark.Call(thread, zip, starlark.Tuple{sequence, sequence}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		})
	})

	t.Run("lazy-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iterable := &testIterable{st.N, nth}

			result, err := starlark.Call(thread, zip, starlark.Tuple{iterable, iterable}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		})
	})

	t.Run("mixed", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			tuple := make(starlark.Tuple, st.N)
			iterable := &testIterable{st.N, nth}
			sequence := &testSequence{st.N * 2, nth}

			result, err := starlark.Call(thread, zip, starlark.Tuple{iterable, sequence, tuple}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		})
	})
}

func TestBytesElemsAllocs(t *testing.T) {
}

func TestDictClearAllocs(t *testing.T) {
	st := startest.From(t)

	dict := starlark.NewDict(100)
	keys := make([]starlark.Value, 100)

	for i := 0; i < 100; i++ {
		keys[i] = starlark.MakeInt(i)
	}

	fn, err := dict.Attr("clear")
	if err != nil {
		st.Fatal(err)
	}

	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			for _, k := range keys {
				dict.SetKey(k, starlark.None)
			}

			_, err := starlark.Call(thread, fn, nil, nil)
			if err != nil {
				st.Fatal(err)
			}
		}
	})
}

func TestDictGetAllocs(t *testing.T) {

	dict := starlark.NewDict(100)
	keys := make([]starlark.Value, 100)

	for i := 0; i < 100; i++ {
		keys[i] = starlark.MakeInt(i)
		dict.SetKey(keys[i], keys[i])
	}

	fn, err := dict.Attr("get")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("present", func(t *testing.T) {
		st := startest.From(t)

		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value, err := starlark.Call(thread, fn, starlark.Tuple{starlark.MakeInt(i % 100)}, nil)
				if err != nil {
					st.Fatal(err)
				}

				if value == starlark.None {
					st.Fatalf("key %v not found", keys[i])
				}

				st.KeepAlive(value)
			}
			st.KeepAlive(dict)
		})
	})

	t.Run("missing", func(t *testing.T) {
		st := startest.From(t)

		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value, err := starlark.Call(thread, fn, starlark.Tuple{starlark.None}, nil)
				if err != nil {
					st.Fatal(err)
				}

				if value != starlark.None {
					st.Fatal("`None` is not a key")
				}

				st.KeepAlive(value)
			}
			st.KeepAlive(dict)
		})
	})
}

func TestDictItemsAllocs(t *testing.T) {
}

func TestDictKeysAllocs(t *testing.T) {
}

func TestDictPopAllocs(t *testing.T) {
	st := startest.From(t)

	dict := starlark.NewDict(100)

	for i := 0; i < 100; i++ {
		key := starlark.MakeInt(i)
		dict.SetKey(key, key)
	}

	fn, err := dict.Attr("pop")

	if err != nil {
		st.Fatal(err)
	}

	if fn == nil {
		st.Fatal("`dict.pop` method doesn't exists")
	}

	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.NotSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			value, err := starlark.Call(thread, fn, starlark.Tuple{starlark.MakeInt(i % 100)}, nil)
			if err != nil {
				st.Fatal(err)
			}

			st.KeepAlive(value)

			dict.SetKey(value, value)
		}
	})
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

func testStringIterable(t *testing.T, methodName string) {
	method, _ := starlark.String("arbitrary-string").Attr(methodName)
	if method == nil {
		t.Errorf("no such method: string.%s", methodName)
		return
	}

	st := startest.From(t)

	st.RequireSafety(starlark.MemSafe)

	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, method, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringCodepointOrdsAllocs(t *testing.T) {
	testStringIterable(t, "codepoint_ords")
}

func TestStringCodepointsAllocs(t *testing.T) {
	testStringIterable(t, "codepoints")
}

func TestStringCountAllocs(t *testing.T) {
}

func TestStringElemOrdsAllocs(t *testing.T) {
	testStringIterable(t, "elem_ords")
}

func TestStringElemsAllocs(t *testing.T) {
	testStringIterable(t, "elems")
}

func TestStringEndswithAllocs(t *testing.T) {
}

func TestStringFindAllocs(t *testing.T) {
}

func TestStringFormatAllocs(t *testing.T) {
	sample := starlark.Tuple{
		nil, // Not a starlark value, but useful for testing
		starlark.None,
		starlark.True,
		starlark.MakeInt(1),
		starlark.MakeInt64(1 << 40),
		starlark.String("\"test\""),
		starlark.NewDict(0),
		starlark.NewSet(0),
		starlark.NewList([]starlark.Value{starlark.False}),
	}

	t.Run("args", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				format := starlark.String("{{{0!s}}}")
				fn, err := format.Attr("format")
				if err != nil {
					st.Error(err)
					return
				}
				if fn == nil {
					st.Errorf("`string.format` builtin doesn't exists")
					return
				}
				result, err := starlark.Call(thread, fn, starlark.Tuple{sample}, nil)
				if err != nil {
					st.Error(err)
					return
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("kwargs", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				format := starlark.String("{{{a!s}}}")
				fn, err := format.Attr("format")
				if err != nil {
					st.Error(err)
					return
				}
				if fn == nil {
					st.Errorf("`string.format` builtin doesn't exists")
					return
				}
				result, err := starlark.Call(thread, fn, nil, []starlark.Tuple{{starlark.String("a"), sample}})
				if err != nil {
					st.Error(err)
					return
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringIndexAllocs(t *testing.T) {
}

func TestStringIsalnumAllocs(t *testing.T) {
	string_isalnum, _ := starlark.String("hello, world!").Attr("isalnum")
	if string_isalnum == nil {
		t.Fatal("no such method: string.isalnum")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_isalnum, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIsalphaAllocs(t *testing.T) {
	string_isalpha, _ := starlark.String("hello, world!").Attr("isalpha")
	if string_isalpha == nil {
		t.Fatal("no such method: string.isalpha")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_isalpha, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIsdigitAllocs(t *testing.T) {
	string_isdigit, _ := starlark.String("1234567890").Attr("isdigit")
	if string_isdigit == nil {
		t.Fatal("no such method: string.isdigit")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_isdigit, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIslowerAllocs(t *testing.T) {
	string_islower, _ := starlark.String("sphinx of black quartz, judge my vow").Attr("islower")
	if string_islower == nil {
		t.Fatal("no such method: string.islower")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_islower, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIsspaceAllocs(t *testing.T) {
	string_isspace, _ := starlark.String("    \t    ").Attr("isspace")
	if string_isspace == nil {
		t.Fatal("no such method: string.isspace")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_isspace, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIstitleAllocs(t *testing.T) {
	string_istitle, _ := starlark.String("Hello, world!").Attr("istitle")
	if string_istitle == nil {
		t.Fatal("no such method: string.istitle")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_istitle, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringIsupperAllocs(t *testing.T) {
}

func TestStringJoinAllocs(t *testing.T) {
}

func TestStringLowerAllocs(t *testing.T) {
}

func testStringStripAllocs(t *testing.T, method_name string) {
	method, _ := starlark.String("     ababaZZZZZababa     ").Attr(method_name)
	if method == nil {
		t.Errorf("no such method: string.%s", method_name)
		return
	}

	t.Run("with-cutset=no", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, method, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-cutset=yes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.String("ab ")}
				result, err := starlark.Call(thread, method, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringLstripAllocs(t *testing.T) {
	testStringStripAllocs(t, "lstrip")
}

func TestStringPartitionAllocs(t *testing.T) {
}

func TestStringRemoveprefixAllocs(t *testing.T) {
}

func TestStringRemovesuffixAllocs(t *testing.T) {
}

func TestStringReplaceAllocs(t *testing.T) {
	st := startest.From(t)

	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		str := starlark.String(strings.Repeat("deadbeef", st.N))
		toReplace := starlark.String("beef")
		replacement := starlark.String("🍖")

		fn, _ := str.Attr("replace")
		if fn == nil {
			st.Fatal("no such method: string.replace")
		}

		result, err := starlark.Call(thread, fn, starlark.Tuple{toReplace, replacement}, nil)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}

func TestStringRfindAllocs(t *testing.T) {
}

func TestStringRindexAllocs(t *testing.T) {
}

func TestStringRpartitionAllocs(t *testing.T) {
}

func TestStringRsplitAllocs(t *testing.T) {
	t.Run("delimiter", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			delimiter := starlark.String("beef")
			// I must count the string content as well since it will
			// be kept alive by the slices taken by the delimeter.
			str := starlark.String(strings.Repeat("deadbeef", st.N))
			if err := thread.AddAllocs(int64(len(str))); err != nil {
				st.Error(err)
			}

			string_rsplit, _ := str.Attr("rsplit")
			if string_rsplit == nil {
				st.Fatal("no such method: string.rsplit")
			}

			result, err := starlark.Call(thread, string_rsplit, starlark.Tuple{delimiter, starlark.MakeInt(st.N)}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("empty-delimiter", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("go    go", st.N))
			if err := thread.AddAllocs(int64(len(str))); err != nil {
				st.Error(err)
			}

			string_split, _ := str.Attr("rsplit")
			if string_split == nil {
				st.Fatal("no such method: string.rsplit")
			}

			result, err := starlark.Call(thread, string_split, starlark.Tuple{starlark.None, starlark.MakeInt(st.N)}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestStringRstripAllocs(t *testing.T) {
	testStringStripAllocs(t, "rstrip")
}

func TestStringSplitAllocs(t *testing.T) {
	t.Run("delimiter", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			delimiter := starlark.String("beef")
			// I must count the string content as well since it will
			// be kept alive by the slices taken by the delimeter.
			str := starlark.String(strings.Repeat("deadbeef", st.N))
			if err := thread.AddAllocs(int64(len(str))); err != nil {
				st.Error(err)
			}

			string_split, _ := str.Attr("split")
			if string_split == nil {
				st.Fatal("no such method: string.split")
			}

			result, err := starlark.Call(thread, string_split, starlark.Tuple{delimiter}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("empty-delimiter", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("go    go", st.N))
			if err := thread.AddAllocs(int64(len(str))); err != nil {
				st.Error(err)
			}

			string_split, _ := str.Attr("split")
			if string_split == nil {
				st.Fatal("no such method: string.split")
			}

			result, err := starlark.Call(thread, string_split, starlark.Tuple{}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String("g g g")
			string_split, _ := str.Attr("split")
			if string_split == nil {
				st.Fatal("no such method: string.split")
			}

			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_split, starlark.Tuple{}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringSplitlinesAllocs(t *testing.T) {
}

func TestStringStartswithAllocs(t *testing.T) {
}

func TestStringStripAllocs(t *testing.T) {
	testStringStripAllocs(t, "strip")
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
func (it *allocatingIterator) Safety() starlark.Safety {
	if it.thread == nil {
		return starlark.NotSafe
	}
	return starlark.MemSafe
}

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

func TestTupleIteration(t *testing.T) {
	values := starlark.Tuple{
		starlark.None,
		starlark.False,
		starlark.True,
		starlark.MakeInt(0),
		starlark.MakeInt64(1 << 34),
		starlark.String("starlark"),
		starlark.NewList(nil),
		starlark.NewDict(10),
	}

	tupleAsValue := starlark.Value(values)

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			it, err := starlark.SafeIterate(thread, tupleAsValue)
			if err != nil {
				st.Fatal(err)
			}
			defer it.Done()

			var v starlark.Value
			for j := 0; it.Next(&v); j++ {
				if v != values[j] {
					st.Errorf("expected %v got %v", values[j], v)
				}
				st.KeepAlive(v)
			}

			if err := it.Err(); err != nil {
				st.Error(err)
			}
		}
	})
}
