package starlark_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

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

type unsafeTestIterable struct {
	testBase startest.TestBase
}

var _ starlark.Iterable = &unsafeTestIterable{}

func (ui *unsafeTestIterable) Freeze() {}
func (ui *unsafeTestIterable) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", ui.Type())
}
func (ui *unsafeTestIterable) String() string       { return "unsafeTestIterable" }
func (ui *unsafeTestIterable) Truth() starlark.Bool { return false }
func (ui *unsafeTestIterable) Type() string         { return "unsafeTestIterable" }
func (ui *unsafeTestIterable) Iterate() starlark.Iterator {
	return &unsafeTestIterator{
		testBase: ui.testBase,
	}
}

type unsafeTestIterator struct {
	testBase startest.TestBase
}

var _ starlark.Iterator = &unsafeTestIterator{}

func (ui *unsafeTestIterator) Next(p *starlark.Value) bool {
	ui.testBase.Error("Next called")
	return false
}
func (ui *unsafeTestIterator) Done()      {}
func (ui *unsafeTestIterator) Err() error { return fmt.Errorf("Err called") }

func TestAbsSteps(t *testing.T) {
	abs, ok := starlark.Universe["abs"]
	if !ok {
		t.Fatal("no such builtin: abs")
	}

	t.Run("const-size", func(t *testing.T) {
		numbers := []starlark.Value{
			starlark.MakeInt(0),
			starlark.MakeInt(-1),
			starlark.MakeInt(-1000),
			starlark.MakeInt64(-(1 << 40)),
			starlark.Float(-1e20),
		}

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		for _, n := range numbers {
			st.RunThread(func(thread *starlark.Thread) {
				_, err := starlark.Call(thread, abs, starlark.Tuple{n}, nil)
				if err != nil {
					st.Error(err)
				}
			})
		}
	})

	t.Run("var-size", func(t *testing.T) {
		t.Run("positive", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.RunThread(func(thread *starlark.Thread) {
				n := starlark.Value(starlark.MakeInt(1).Lsh(uint(st.N)))
				starlark.Call(thread, abs, nil, nil) // warmup
				st.ResetTimer()
				_, err := starlark.Call(thread, abs, starlark.Tuple{n}, nil)
				if err != nil {
					st.Error(err)
				}
			})
		})

		t.Run("negative", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMaxExecutionSteps(1)
			st.SetMinExecutionSteps(0)
			st.RunThread(func(thread *starlark.Thread) {
				n := starlark.MakeInt(-1).Lsh(uint(st.N))
				starlark.Call(thread, abs, nil, nil) // warmup
				st.ResetTimer()
				_, err := starlark.Call(thread, abs, starlark.Tuple{n}, nil)
				if err != nil {
					st.Error(err)
				}
			})
		})
	})
}

func TestAbsAllocs(t *testing.T) {
	abs, ok := starlark.Universe["abs"]
	if !ok {
		t.Fatal("no such builtin: abs")
	}

	t.Run("positive-ints", func(t *testing.T) {
		st := startest.From(t)

		var one starlark.Value = starlark.MakeInt(1)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, abs, starlark.Tuple{one}, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})

	t.Run("small-ints", func(t *testing.T) {
		st := startest.From(t)

		var speedOfLightInVacuum starlark.Value = starlark.MakeInt(-299792458)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, abs, starlark.Tuple{speedOfLightInVacuum}, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})

	t.Run("big-ints", func(t *testing.T) {
		st := startest.From(t)

		var electrostaticConstant starlark.Value = starlark.MakeInt64(-8987551792)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, abs, starlark.Tuple{electrostaticConstant}, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})

	t.Run("positive-floats", func(t *testing.T) {
		st := startest.From(t)

		var pi starlark.Value = starlark.Float(math.Pi)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, abs, starlark.Tuple{pi}, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})

	t.Run("floats", func(t *testing.T) {
		st := startest.From(t)

		var electronElementaryCharge starlark.Value = starlark.Float(-1.602176634e-19)

		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, abs, starlark.Tuple{electronElementaryCharge}, nil)
				if err != nil {
					st.Error(err)
				}

				st.KeepAlive(result)
			}
		})
	})
}

func TestAnySteps(t *testing.T) {
}

func TestAnyAllocs(t *testing.T) {
	any, ok := starlark.Universe["any"]
	if !ok {
		t.Fatal("no such builtin: any")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)
		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, any, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("result", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{&testIterable{
					maxN: 10,
					nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
						return starlark.False, nil
					},
				}}
				result, err := starlark.Call(thread, any, args, nil)
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
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					overheadSize := starlark.EstimateMakeSize([]starlark.Value{}, 16) +
						starlark.EstimateSize(starlark.List{})
					if err := thread.AddAllocs(overheadSize); err != nil {
						return nil, err
					}
					st.KeepAlive(starlark.NewList(make([]starlark.Value, 0, 16)))

					return starlark.False, nil
				}},
			}
			result, err := starlark.Call(thread, any, args, nil)
			if err != nil {
				t.Errorf("unexpected error: %s", err.Error())
			}
			st.KeepAlive(result)
		})
	})
}

func TestAllSteps(t *testing.T) {
}

func TestAllAllocs(t *testing.T) {
	all, ok := starlark.Universe["all"]
	if !ok {
		t.Fatal("no such builtin: all")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, all, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

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

func TestBoolSteps(t *testing.T) {
	bool_, ok := starlark.Universe["bool"]
	if !ok {
		t.Fatal("no such builtin: bool")
	}

	t.Run("const-size", func(t *testing.T) {
		tests := []struct {
			name string
			args starlark.Tuple
		}{
			{
				name: "none",
				args: starlark.Tuple{starlark.None},
			}, {
				name: "true",
				args: starlark.Tuple{starlark.True},
			}, {
				name: "int",
				args: starlark.Tuple{starlark.MakeInt(0)},
			}, {
				name: "float",
				args: starlark.Tuple{starlark.Float(0.5)},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
			st.RunThread(func(thread *starlark.Thread) {
					_, err := starlark.Call(thread, bool_, test.args, nil)
				if err != nil {
					st.Error(err)
				}
				})
			})
		}
	})

	t.Run("var-size", func(t *testing.T) {
		generators := []struct {
			name string
			fn   func(n int) starlark.Value
		}{
			{
				"big-int",
				func(n int) starlark.Value {
					return starlark.MakeInt(1).Lsh(uint(n))
				},
			},
			{
				"string",
				func(n int) starlark.Value {
					return starlark.String(strings.Repeat("a", n))
				},
			},
			{
				"set",
				func() func(int) starlark.Value {
					set := starlark.NewSet(0)
					return func(n int) starlark.Value {
						for i := set.Len(); i < n; i++ {
						set.Insert(starlark.MakeInt(i))
					}
					return set
					}
				}(),
			},
			{
				"dict",
				func() func(int) starlark.Value {
					dict := starlark.NewDict(0)
					return func(n int) starlark.Value {
						for i := dict.Len(); i < n; i++ {
						dict.SetKey(starlark.MakeInt(i), starlark.None)
					}
					return dict
					}
				}(),
			},
			{
				"list",
				func() func(int) starlark.Value {
					list := starlark.NewList([]starlark.Value{})
					return func(n int) starlark.Value {
						for i := list.Len(); i < n; i++ {
							list.Append(starlark.None)
					}
					return list
					}
				}(),
			},
		}

		for _, gen := range generators {
			t.Run(gen.name, func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.CPUSafe)
				st.RunThread(func(thread *starlark.Thread) {
					value := gen.fn(st.N)
					starlark.Call(thread, bool_, nil, nil) // warmup
					st.ResetTimer()
					_, err := starlark.Call(thread, bool_, starlark.Tuple{value}, nil)
					if err != nil {
						st.Error(err)
					}
				})
			})
		}
	})
}

func TestBoolAllocs(t *testing.T) {
	bool_, ok := starlark.Universe["bool"]
	if !ok {
		t.Fatal("no such builtin: bool")
	}

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

func TestBytesSteps(t *testing.T) {
}

func TestBytesAllocs(t *testing.T) {
	bytes, ok := starlark.Universe["bytes"]
	if !ok {
		t.Fatal("No such builtin: bytes")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)
		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, bytes, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.Bytes("foobar")}
				result, err := starlark.Call(thread, bytes, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("small-valid-string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String("hello, world!")
			result, err := starlark.Call(thread, bytes, starlark.Tuple{str}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("large-valid-string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("hello, world!", st.N))
			if err := thread.AddAllocs(starlark.EstimateSize(str)); err != nil {
				st.Error(err)
			}
			st.KeepAlive(str)

			result, err := starlark.Call(thread, bytes, starlark.Tuple{str}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("small-invalid-string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			testString := string([]byte{0x80, 0x80, 0x80, 0x80, 0x80})
			if utf8.ValidString(testString) {
				st.Fatal("test string will not force allocations")
			}
			if err := thread.AddAllocs(starlark.EstimateSize(testString)); err != nil {
				st.Error(err)
			}
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{starlark.String(testString)}
				result, err := starlark.Call(thread, bytes, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("large-invalid-string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			testBytes := []byte{}
			for i := 0; i < st.N; i++ {
				testBytes = append(testBytes, 0x80)
			}
			testString := string(testBytes)
			if utf8.ValidString(testString) {
				st.Fatal("test string will not force allocations")
			}
			args := starlark.Tuple{starlark.String(testString)}
			result, err := starlark.Call(thread, bytes, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					overheadSize := starlark.EstimateMakeSize([]byte{}, 100) + starlark.SliceTypeOverhead
					if err := thread.AddAllocs(overheadSize); err != nil {
						return nil, err
					}
					st.KeepAlive(make([]byte, 100))
					return starlark.MakeInt(n % 256), nil
				},
				maxN: st.N,
			}
			result, err := starlark.Call(thread, bytes, starlark.Tuple{iter}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestChrSteps(t *testing.T) {
	chr, ok := starlark.Universe["chr"]
	if !ok {
		t.Fatal("no such builtin: chr")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.RunThread(func(thread *starlark.Thread) {
		_, err := starlark.Call(thread, chr, starlark.Tuple{starlark.MakeInt(st.N)}, nil)
		if err != nil {
			st.Error(err)
		}
	})
}

func TestChrAllocs(t *testing.T) {
	chr, ok := starlark.Universe["chr"]
	if !ok {
		t.Fatal("no such builtin: chr")
	}

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

func TestDictSteps(t *testing.T) {
}

func TestDictAllocs(t *testing.T) {
	dict, ok := starlark.Universe["dict"]
	if !ok {
		t.Fatal("no such builtin: dict")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "dict: feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, dict, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	values := &testIterable{
		nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
			var result starlark.Value = starlark.Tuple{starlark.MakeInt(n), starlark.None}
			if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
				return nil, err
			}
			return result, nil
		},
		maxN: 100,
	}

	t.Run("fixed", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			values.maxN = 100
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, dict, starlark.Tuple{values}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("average", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			values.maxN = st.N
			result, err := starlark.Call(thread, dict, starlark.Tuple{values}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestDirSteps(t *testing.T) {
}

func TestDirAllocs(t *testing.T) {
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

	dir, ok := starlark.Universe["dir"]
	if !ok {
		t.Fatal("no such builtin: dir")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	for _, value := range values {
		st.RunThread(func(thread *starlark.Thread) {
			result, err := starlark.Call(thread, dir, starlark.Tuple{value}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	}
}

func TestEnumerateSteps(t *testing.T) {
}

func TestEnumerateAllocs(t *testing.T) {
	enumerate, ok := starlark.Universe["enumerate"]
	if !ok {
		t.Fatal("no such builtin: enumerate")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, enumerate, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

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

func TestFailSteps(t *testing.T) {
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

	fail, ok := starlark.Universe["fail"]
	if !ok {
		t.Fatal("no such builtin: fail")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			res, err := starlark.Call(thread, fail, args, nil)
			if err == nil {
				st.Errorf("fail returned success %v", res)
			}
			st.KeepAlive(err.Error())
			thread.AddAllocs(starlark.StringTypeOverhead)
		}
	})
}

func TestFloatSteps(t *testing.T) {
	float, ok := starlark.Universe["float"]
	if !ok {
		t.Fatal("no such builtin: float")
	}

	t.Run("const-size", func(t *testing.T) {
		values := []starlark.Value{
			starlark.True,
			starlark.MakeInt(0),
			starlark.Float(-1),
		}

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		for _, value := range values {
			st.RunThread(func(thread *starlark.Thread) {
				_, err := starlark.Call(thread, float, starlark.Tuple{value}, nil)
				if err != nil {
					st.Error(err)
				}
			})
		}
	})

	t.Run("var-size", func(t *testing.T) {
		t.Run("int", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.RunThread(func(thread *starlark.Thread) {
				number := starlark.Value(starlark.MakeInt(1).Lsh(uint(st.N)))
				starlark.Call(thread, float, nil, nil) // warmup
				st.ResetTimer()
				// This code is not checking for the error as it *will*
				// error when `number`` is big enough. The execution must
				// still be checked to make sure that before getting that
				// error it doesn't consumes too much cpu (as it was doing
				// before).
				starlark.Call(thread, float, starlark.Tuple{number}, nil)
			})
		})

		t.Run("string", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMinExecutionSteps(1)
			st.SetMaxExecutionSteps(1)
			st.RunThread(func(thread *starlark.Thread) {
				number := starlark.Value(starlark.String(strings.Repeat("0", st.N)))
				starlark.Call(thread, float, nil, nil) // warmup
				st.ResetTimer()
				_, err := starlark.Call(thread, float, starlark.Tuple{number}, nil)
				if err != nil {
					st.Error(err)
				}
			})
		})
	})
}

func TestFloatAllocs(t *testing.T) {
	float, ok := starlark.Universe["float"]
	if !ok {
		t.Fatal("no such builtin: float")
	}

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

func TestGetattrSteps(t *testing.T) {
}

func TestGetattrAllocs(t *testing.T) {
}

func TestHasattrSteps(t *testing.T) {
}

func TestHasattrAllocs(t *testing.T) {
	hasattr, ok := starlark.Universe["hasattr"]
	if !ok {
		t.Fatal("no such builtin: hasattr")
	}

	recvs := []starlark.HasAttrs{
		starlark.String(""),
		starlark.NewDict(0),
		starlark.NewList(nil),
		starlark.NewSet(0),
		starlark.Bytes(""),
	}

	t.Run("missing-attr", func(t *testing.T) {
		missing := starlark.String("solve_non_polynomial")

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		for _, obj := range recvs {
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					result, err := starlark.Call(thread, hasattr, starlark.Tuple{obj, missing}, nil)
					if err != nil {
						st.Error(err)
					}
					if result != starlark.False {
						st.Error("missing method is present")
					}
					st.KeepAlive(result)
				}
			})
		}
	})

	t.Run("existent-attr", func(t *testing.T) {
		st := startest.From(t)
		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		for _, recv := range recvs {
			st.RunThread(func(thread *starlark.Thread) {
				for i := 0; i < st.N; i++ {
					result, err := starlark.Call(thread, hasattr, starlark.Tuple{recv, starlark.String(recv.AttrNames()[0])}, nil)
					if err != nil {
						st.Error(err)
					}
					if result != starlark.True {
						st.Error("declared method is not present")
					}
					st.KeepAlive(result)
				}
			})
		}
	})
}

func TestHashSteps(t *testing.T) {
	hash, ok := starlark.Universe["hash"]
	if !ok {
		t.Fatal("no such builtin: hash")
	}

	t.Run("input=string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinExecutionSteps(16)
		st.SetMaxExecutionSteps(16)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String(strings.Repeat("a", st.N*16))}
			st.ResetTimer()
			_, err := starlark.Call(thread, hash, args, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("input=bytes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinExecutionSteps(16)
		st.SetMaxExecutionSteps(16)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.Bytes(strings.Repeat("a", st.N*16))}
			st.ResetTimer()
			_, err := starlark.Call(thread, hash, args, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})
}

func TestHashAllocs(t *testing.T) {
	hash, ok := starlark.Universe["hash"]
	if !ok {
		t.Fatal("no such builtin: hash")
	}

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

func TestIntSteps(t *testing.T) {
	int_, ok := starlark.Universe["int"]
	if !ok {
		t.Fatal("no such builtin: int")
	}

	t.Run("bool", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			_, err := starlark.Call(thread, int_, starlark.Tuple{starlark.False}, nil)
			if err != nil {
				st.Error(err)
			}
			_, err = starlark.Call(thread, int_, starlark.Tuple{starlark.True}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("float", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			f := math.Pow(2, float64(st.N))
			if math.IsInf(f, 0) {
				f = math.MaxFloat64
			}
			st.ResetTimer()
			_, err := starlark.Call(thread, int_, starlark.Tuple{starlark.Float(f)}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("int", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			n := starlark.Value(starlark.MakeInt(1).Lsh(uint(st.N)))
			starlark.Call(thread, int_, nil, nil) // warmup
			st.ResetTimer()
			_, err := starlark.Call(thread, int_, starlark.Tuple{n}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinExecutionSteps(16)
		st.SetMaxExecutionSteps(16)
		st.RunThread(func(thread *starlark.Thread) {
			n := starlark.String(strings.Repeat("0", st.N*16))
			st.ResetTimer()
			_, err := starlark.Call(thread, int_, starlark.Tuple{n}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})
}

func TestIntAllocs(t *testing.T) {
	int_, ok := starlark.Universe["int"]
	if !ok {
		t.Fatal("no such builtin: int")
	}

	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(th *starlark.Thread) {
			inputString := starlark.String("deadbeef")
			args := []starlark.Value{inputString, starlark.MakeInt(16)}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(th, int_, args, nil)
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
			result, err := starlark.Call(th, int_, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestLenSteps(t *testing.T) {
	len_, ok := starlark.Universe["len"]
	if !ok {
		t.Fatal("no such builtin: len")
	}

	t.Run("string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			value := starlark.String(strings.Repeat("a", st.N))
			starlark.Call(thread, len_, nil, nil)
			st.ResetTimer()
			_, err := starlark.Call(thread, len_, starlark.Tuple{value}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("indexable", func(t *testing.T) {
		value := starlark.NewList([]starlark.Value{})
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := value.Len(); i < st.N; i++ {
				value.Append(starlark.None)
			}
			st.ResetTimer()
			_, err := starlark.Call(thread, len_, starlark.Tuple{value}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("sequence", func(t *testing.T) {
		value := starlark.NewDict(0)
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := value.Len(); i < st.N; i++ {
				value.SetKey(starlark.MakeInt(i), starlark.None)
			}
			st.ResetTimer()
			_, err := starlark.Call(thread, len_, starlark.Tuple{value}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})
}

func TestLenAllocs(t *testing.T) {
	len_, ok := starlark.Universe["len"]
	if !ok {
		t.Fatal("no such builtin: len")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, len_, starlark.Tuple{starlark.String("test")}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestListSteps(t *testing.T) {
}

func TestListAllocs(t *testing.T) {
	list, ok := starlark.Universe["list"]
	if !ok {
		t.Fatal("no such builtin: list")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)
		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, list, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("small-iterable", func(t *testing.T) {
		const numTestElems = 10

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
					result := starlark.Value(starlark.NewList(nil))
					if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				},
				maxN: numTestElems,
			}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, list, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("big-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
					result := starlark.Value(starlark.NewList(nil))
					if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				},
				maxN: st.N,
			}
			result, err := starlark.Call(thread, list, starlark.Tuple{iter}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("small-sequence", func(t *testing.T) {
		const numTestElems = 10

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testSequence{
				nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
					result := starlark.Value(starlark.NewList(nil))
					if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				},
				maxN: numTestElems,
			}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, list, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("big-sequence", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testSequence{
				nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
					result := starlark.Value(starlark.NewList(nil))
					if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
						return nil, err
					}
					return result, nil
				},
				maxN: st.N,
			}
			result, err := starlark.Call(thread, list, starlark.Tuple{iter}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func testMinMaxSteps(t *testing.T) {
}

func testMinMaxAllocs(t *testing.T, name string) {
	minOrMax, ok := starlark.Universe[name]
	if !ok {
		t.Fatalf("no such builtin: %s", name)
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, minOrMax, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("result", func(t *testing.T) {
		iterable := &testIterable{
			nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
				res := starlark.Value(starlark.MakeInt(n))
				if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
					return nil, err
				}
				return res, nil
			},
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iterable.maxN = st.N

			result, err := starlark.Call(thread, minOrMax, starlark.Tuple{iterable}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestMaxSteps(t *testing.T) {
}

func TestMaxAllocs(t *testing.T) {
	testMinMaxAllocs(t, "max")
}

func TestMinSteps(t *testing.T) {
}

func TestMinAllocs(t *testing.T) {
	testMinMaxAllocs(t, "min")
}

func TestOrdSteps(t *testing.T) {
}

func TestOrdAllocs(t *testing.T) {
	ord, ok := starlark.Universe["ord"]
	if !ok {
		t.Fatal("no such builtin: ord")
	}

	t.Run("input=string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String("Д")}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, ord, args, nil)
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
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.Bytes("d")}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, ord, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestPrintSteps(t *testing.T) {
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

	print, ok := starlark.Universe["print"]
	if !ok {
		t.Fatal("no such builtin: print")
	}

	st := startest.From(t)
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
			res, err := starlark.Call(thread, print, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(res)
		}
	})
}

func TestRangeSteps(t *testing.T) {
}

func TestRangeAllocs(t *testing.T) {
	range_, ok := starlark.Universe["range"]
	if !ok {
		t.Fatal("no such builtin: range")
	}

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

func TestReprSteps(t *testing.T) {
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

	repr, ok := starlark.Universe["repr"]
	if !ok {
		t.Fatal("no such builtin: repr")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			res, err := starlark.Call(thread, repr, starlark.Tuple{args}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(res)
		}
	})
}

func TestReversedSteps(t *testing.T) {
}

func TestReversedAllocs(t *testing.T) {
	reversed, ok := starlark.Universe["reversed"]
	if !ok {
		t.Fatal("no such builtin: reversed")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, reversed, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("small-result", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)

		iter := &testIterable{
			maxN: 10,
			nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
				res := starlark.Value(starlark.Tuple(make([]starlark.Value, 100)))
				if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
					return nil, err
				}
				return res, nil
			},
		}
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				args := starlark.Tuple{iter}
				result, err := starlark.Call(thread, reversed, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("large-result", func(t *testing.T) {
		t.Run("iterable", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				iter := &testIterable{
					maxN: st.N,
					nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
						res := starlark.Value(starlark.Tuple(make([]starlark.Value, 100)))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}

				result, err := starlark.Call(thread, reversed, starlark.Tuple{iter}, nil)
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
						res := starlark.Value(starlark.Tuple(make([]starlark.Value, 100)))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}

				result, err := starlark.Call(thread, reversed, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			})
		})
	})

	t.Run("early-termination", func(t *testing.T) {
		const expected = "exceeded memory allocation limits"
		maxAllocs := uint64(50)

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
						res := starlark.Value(starlark.Tuple(make([]starlark.Value, maxAllocs/2)))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						nReached = n
						return res, nil
					},
				}

				result, err := starlark.Call(thread, reversed, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				st.KeepAlive(result)

				if nReached > 1 && iter.maxN > 1 {
					st.Errorf("iteration was not terminated early enough")
				}
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
						res := starlark.Value(starlark.Tuple(make([]starlark.Value, maxAllocs/2)))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						nReached = n
						return res, nil
					},
				}

				result, err := starlark.Call(thread, reversed, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				st.KeepAlive(result)

				if nReached > 0 {
					st.Errorf("iteration was not terminated early enough")
				}
			})
		})
	})
}

func TestSetSteps(t *testing.T) {
}

func TestSetAllocs(t *testing.T) {
	set, ok := starlark.Universe["set"]
	if !ok {
		t.Fatal("no such builtin: set")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		iter := &testIterable{
			nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
				res := starlark.Value(starlark.MakeInt(n))
				if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
					return nil, err
				}
				return res, nil
			},
			maxN: st.N,
		}
		result, err := starlark.Call(thread, set, starlark.Tuple{iter}, nil)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}

func TestSortedSteps(t *testing.T) {
}

func TestSortedAllocs(t *testing.T) {
	sorted, ok := starlark.Universe["sorted"]
	if !ok {
		t.Fatal("no such builtin: sorted")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, sorted, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("small-result", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				maxN: 10,
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					res := starlark.MakeInt(-n)
					if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
						return nil, err
					}
					return res, nil
				},
			}
			args := starlark.Tuple{iter}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, sorted, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("large-result", func(t *testing.T) {
		t.Run("iterable", func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				iter := &testIterable{
					maxN: st.N,
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						res := starlark.MakeInt(-n)
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}

				result, err := starlark.Call(thread, sorted, starlark.Tuple{iter}, nil)
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
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						res := starlark.MakeInt(-n)
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}

				result, err := starlark.Call(thread, sorted, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			})
		})
	})

	t.Run("early-termination", func(t *testing.T) {
		const expected = "exceeded memory allocation limits"
		maxAllocs := uint64(1)

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
						res := starlark.MakeInt(-n)
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						nReached = n
						return res, nil
					},
				}

				result, err := starlark.Call(thread, sorted, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				st.KeepAlive(result)

				if nReached > 1 && iter.maxN > 1 {
					st.Errorf("iteration was not terminated early enough")
				}
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
						res := starlark.MakeInt(-n)
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}

				result, err := starlark.Call(thread, sorted, starlark.Tuple{iter}, nil)
				if err == nil {
					st.Error("expected error")
				} else if err.Error() != expected {
					st.Errorf("unexpected error: %v", err)
				}
				st.KeepAlive(result)

				if nReached > 0 {
					st.Errorf("iteration was not terminated early enough")
				}
			})
		})
	})
}

func TestStrSteps(t *testing.T) {
}

func TestStrAllocs(t *testing.T) {
	listLoopContent := []starlark.Value{nil}
	var listLoop starlark.Value = starlark.NewList(listLoopContent)
	listLoopContent[0] = listLoop

	dictLoop := starlark.NewDict(1)
	var dictLoopValue starlark.Value = dictLoop
	dictLoop.SetKey(starlark.MakeInt(0x1CEB00DA), dictLoopValue)

	str, ok := starlark.Universe["str"]
	if !ok {
		t.Fatal("no such builtin: str")
	}

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
				res, err := starlark.Call(thread, str, starlark.Tuple{starlark.String("any string `\"'")}, nil)
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
				res, err := starlark.Call(thread, str, starlark.Tuple{args}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(res)
			}
		})
	})
}

func TestTupleSteps(t *testing.T) {
}

func TestTupleAllocs(t *testing.T) {
	tuple, ok := starlark.Universe["tuple"]
	if !ok {
		t.Fatal("no such builtin: tuple")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, tuple, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("small-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				iter := &testIterable{
					maxN: 10,
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						res := starlark.Value(starlark.MakeInt(n))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}
				args := starlark.Tuple{iter}
				result, err := starlark.Call(thread, tuple, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("small-sequence", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				seq := &testSequence{
					maxN: 10,
					nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
						res := starlark.Value(starlark.MakeInt(n))
						if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
							return nil, err
						}
						return res, nil
					},
				}
				args := starlark.Tuple{seq}
				result, err := starlark.Call(thread, tuple, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("large-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				maxN: st.N,
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					res := starlark.Value(starlark.MakeInt(n))
					if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
						return nil, err
					}
					return res, nil
				},
			}
			args := starlark.Tuple{iter}
			result, err := starlark.Call(thread, tuple, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("large-sequence", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			seq := &testSequence{
				maxN: st.N,
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					res := starlark.Value(starlark.MakeInt(n))
					if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
						return nil, err
					}
					return res, nil
				},
			}
			args := starlark.Tuple{seq}
			result, err := starlark.Call(thread, tuple, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("early-termination", func(t *testing.T) {
		const expected = "exceeded memory allocation limits"
		maxAllocs := uint64(30)

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

				result, err := starlark.Call(thread, tuple, starlark.Tuple{iter}, nil)
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

				result, err := starlark.Call(thread, tuple, starlark.Tuple{iter}, nil)
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

func TestTypeSteps(t *testing.T) {
}

func TestTypeAllocs(t *testing.T) {
	type_, ok := starlark.Universe["type"]
	if !ok {
		t.Fatal("no such builtin: type")
	}

	values := []starlark.Value{
		starlark.None,
		starlark.True,
		starlark.MakeInt(1),
		starlark.MakeInt64(1 << 40),
		starlark.String("\"test\""),
		starlark.NewDict(0),
		starlark.NewSet(0),
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	for _, value := range values {
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, type_, starlark.Tuple{value}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	}
}

func TestZipSteps(t *testing.T) {
}

func TestZipAllocs(t *testing.T) {
	zip, ok := starlark.Universe["zip"]
	if !ok {
		t.Fatal("no such builtin: zip")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, zip, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

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

func TestBytesElemsSteps(t *testing.T) {
}

func TestBytesElemsAllocs(t *testing.T) {
	t.Run("iterator-acquisition", func(t *testing.T) {
		bytes_elems, _ := starlark.Bytes("arbitrary-string").Attr("elems")
		if bytes_elems == nil {
			t.Fatal("no such method: bytes.elems")
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, bytes_elems, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("iterator-usage", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			bytes_elems, _ := starlark.Bytes(strings.Repeat("hello world", st.N)).Attr("elems")
			if bytes_elems == nil {
				st.Fatal("no such method: bytes.elems")
			}
			elems, err := starlark.Call(thread, bytes_elems, nil, nil)
			if err != nil {
				st.Fatal(err)
			}
			iter, err := starlark.SafeIterate(thread, elems)
			if err != nil {
				st.Fatal(err)
			}
			defer iter.Done()
			var x starlark.Value
			for iter.Next(&x) {
				st.KeepAlive(x)
			}
			if err := iter.Err(); err != nil {
				st.Error(err)
			}
		})
	})
}

func TestDictClearSteps(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.SetMinExecutionSteps(1)
	st.SetMaxExecutionSteps(2) // Load factor can lead to up to twice the size
	st.RunThread(func(thread *starlark.Thread) {
		dict := starlark.NewDict(st.N)
		for i := 0; i < st.N*4; i++ { // each bucket has space for 4 elements
			dict.SetKey(starlark.MakeInt(i), starlark.None)
		}
		dict_clear, _ := dict.Attr("clear")
		if dict_clear == nil {
			t.Fatal("no such method: dict.clear")
		}
		st.ResetTimer()
		_, err := starlark.Call(thread, dict_clear, nil, nil)
		if err != nil {
			st.Error(err)
		}
	})
}

func TestDictClearAllocs(t *testing.T) {
	dict := starlark.NewDict(100)
	keys := make([]starlark.Value, 100)
	for i := 0; i < 100; i++ {
		keys[i] = starlark.MakeInt(i)
	}

	dict_clean, err := dict.Attr("clear")
	if err != nil {
		t.Fatal(err)
	}

	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			for _, k := range keys {
				dict.SetKey(k, starlark.None)
			}
			result, err := starlark.Call(thread, dict_clean, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestDictGetSteps(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		dict := starlark.NewDict(0)
			dict_get, _ := dict.Attr("get")
			if dict_get == nil {
				t.Fatal("no such method: dict.get")
			}
		st := startest.From(t)
		st.SetMaxExecutionSteps(1)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := dict.Len(); i < st.N; i++ {
				dict.SetKey(starlark.Float(i), starlark.None)
			}
			elem := starlark.Value(starlark.MakeInt(st.N / 2))
			st.ResetTimer()
			_, err := starlark.Call(thread, dict_get, starlark.Tuple{elem}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("missing", func(t *testing.T) {
		dict := starlark.NewDict(0)
		dict_get, _ := dict.Attr("get")
		if dict_get == nil {
			t.Fatal("no such method: dict.get")
		}
		st := startest.From(t)
		st.SetMaxExecutionSteps(1)
		st.RequireSafety(starlark.CPUSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := dict.Len(); i < st.N; i++ {
				dict.SetKey(starlark.MakeInt(i), starlark.None)
			}
			elem := starlark.MakeInt(st.N)
			st.ResetTimer()
			_, err := starlark.Call(thread, dict_get, starlark.Tuple{elem}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})
}

func TestDictGetAllocs(t *testing.T) {
	dict := starlark.NewDict(100)
	keys := make([]starlark.Value, 100)
	for i := 0; i < 100; i++ {
		keys[i] = starlark.MakeInt(i)
		dict.SetKey(keys[i], keys[i])
	}

	dict_get, _ := dict.Attr("get")
	if dict_get == nil {
		t.Fatal("no such method: dict.get")
	}

	t.Run("present", func(t *testing.T) {
		st := startest.From(t)
		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				value, err := starlark.Call(thread, dict_get, starlark.Tuple{starlark.MakeInt(i % 100)}, nil)
				if err != nil {
					st.Error(err)
				}
				if value == starlark.None {
					st.Errorf("key %v not found", keys[i])
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
				value, err := starlark.Call(thread, dict_get, starlark.Tuple{starlark.None}, nil)
				if err != nil {
					st.Error(err)
				}
				if value != starlark.None {
					st.Error("`None` is not a key")
				}
				st.KeepAlive(value)
			}
			st.KeepAlive(dict)
		})
	})
}

func TestDictItemsSteps(t *testing.T) {
}

func TestDictItemsAllocs(t *testing.T) {
	t.Run("average", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			dict := starlark.NewDict(st.N)
			for i := 0; i < st.N; i++ {
				key := starlark.MakeInt(i)
				dict.SetKey(key, starlark.None)
				if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
					st.Error(err)
				}
			}

			dict_items, _ := dict.Attr("items")
			if dict_items == nil {
				st.Fatal("no such method: dict.items")
			}

			result, err := starlark.Call(thread, dict_items, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("fixed", func(t *testing.T) {
		dict := starlark.NewDict(100)
		for i := 0; i < 100; i++ {
			key := starlark.MakeInt(i)
			dict.SetKey(key, starlark.None)
		}

		dict_items, _ := dict.Attr("items")
		if dict_items == nil {
			t.Fatal("no such method: dict.items")
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, dict_items, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestDictKeysSteps(t *testing.T) {
}

func TestDictKeysAllocs(t *testing.T) {
	t.Run("average", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			dict := starlark.NewDict(st.N)
			for i := 0; i < st.N; i++ {
				key := starlark.MakeInt(i)
				dict.SetKey(key, starlark.None)
				if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
					st.Fatal(err)
				}
			}

			dict_keys, _ := dict.Attr("keys")
			if dict_keys == nil {
				st.Fatal("no such method: dict.keys")
			}

			result, err := starlark.Call(thread, dict_keys, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("fixed", func(t *testing.T) {
		dict := starlark.NewDict(100)
		for i := 0; i < 100; i++ {
			key := starlark.MakeInt(i)
			dict.SetKey(key, starlark.None)
		}

		dict_keys, _ := dict.Attr("keys")
		if dict_keys == nil {
			t.Fatal("no such method: dict.keys")
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, dict_keys, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestDictPopSteps(t *testing.T) {
	t.Run("last", func(t *testing.T) {
		dict := starlark.NewDict(0)
		dict_pop, _ := dict.Attr("pop")
		if dict_pop == nil {
			t.Fatal("no such method: dict.pop")
		}
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
		st.SetMaxExecutionSteps(1)
	st.RunThread(func(thread *starlark.Thread) {
			for i := dict.Len(); i < st.N; i++ {
			dict.SetKey(starlark.MakeInt(i), starlark.None)
		}
			st.ResetTimer()
			_, err := starlark.Call(thread, dict_pop, starlark.Tuple{starlark.MakeInt(st.N), starlark.None}, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})

	t.Run("middle", func(t *testing.T) {
		dict := starlark.NewDict(0)
		dict_pop, _ := dict.Attr("pop")
		if dict_pop == nil {
			t.Fatal("no such method: dict.pop")
		}
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxExecutionSteps(1)
		st.RunThread(func(thread *starlark.Thread) {
			for i := dict.Len(); i <= st.N; i++ {
				dict.SetKey(starlark.MakeInt(i), starlark.None)
			}
			elem := starlark.Value(starlark.MakeInt(st.N / 2))
		st.ResetTimer()
			_, err := starlark.Call(thread, dict_pop, starlark.Tuple{elem}, nil)
		if err != nil {
			st.Error(err)
		}
			st.StopTimer()
			dict.SetKey(elem, starlark.None)
		})
	})
}

func TestDictPopAllocs(t *testing.T) {
	dict := starlark.NewDict(100)
	for i := 0; i < 100; i++ {
		key := starlark.MakeInt(i)
		dict.SetKey(key, key)
	}

	dict_pop, _ := dict.Attr("pop")
	if dict_pop == nil {
		t.Fatal("no such method: dict.pop")
	}

	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, dict_pop, starlark.Tuple{starlark.MakeInt(i % 100)}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
			dict.SetKey(result, result)
		}
	})
}

func TestDictPopitemSteps(t *testing.T) {
	dict := starlark.NewDict(100)
	dict_popitem, _ := dict.Attr("popitem")
	if dict_popitem == nil {
		t.Fatal("no such method: dict.popitem")
	}
	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := dict.Len(); i <= st.N; i++ {
			dict.SetKey(starlark.MakeInt(i), starlark.None)
		}
		st.ResetTimer()
		_, err := starlark.Call(thread, dict_popitem, nil, nil)
		if err != nil {
			st.Error(err)
		}
	})
}

func TestDictPopitemAllocs(t *testing.T) {
	dict := starlark.NewDict(100)
	for i := 0; i < 100; i++ {
		key := starlark.MakeInt(i)
		dict.SetKey(key, key)
	}

	dict_popitem, _ := dict.Attr("popitem")
	if dict_popitem == nil {
		t.Fatal("no such method: dict.popitem")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, dict_popitem, nil, nil)
			if err != nil {
				st.Error(err)
			}
			if tuple, ok := result.(starlark.Tuple); !ok {
				st.Errorf("expected Tuple got %v", tuple.Type())
			} else if tuple.Len() != 2 {
				st.Error("expected a pair")
			} else {
				dict.SetKey(tuple[0], tuple[1])
			}
			st.KeepAlive(result)
		}
		st.KeepAlive(dict)
	})
}

func TestDictSetdefaultSteps(t *testing.T) {
}

func TestDictSetdefaultAllocs(t *testing.T) {
	dict := starlark.NewDict(0)
	dict_setdefault, _ := dict.Attr("setdefault")
	if dict_setdefault == nil {
		t.Fatal("no such method: dict.setdefault")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			var key starlark.Value = starlark.MakeInt(i)
			if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
				st.Error(err)
			}
			result, err := starlark.Call(thread, dict_setdefault, starlark.Tuple{key, key}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}

		st.KeepAlive(dict)
	})
}

func TestDictUpdateSteps(t *testing.T) {
}

func TestDictUpdateAllocs(t *testing.T) {
	dict := starlark.NewDict(0)
	dict_update, _ := dict.Attr("update")
	if dict_update == nil {
		t.Fatal("no such method: dict.update")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "update: feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)

		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, dict_update, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("delta", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				var kv starlark.Value = starlark.MakeInt(i)
				if err := thread.AddAllocs(starlark.EstimateSize(kv)); err != nil {
					st.Error(err)
				}

				updates := starlark.Tuple{starlark.Tuple{kv, kv}, starlark.Tuple{kv, kv}}
				result, err := starlark.Call(thread, dict_update, starlark.Tuple{updates}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
			st.KeepAlive(dict)
		})
	})
}

func TestDictValuesSteps(t *testing.T) {
}

func TestDictValuesAllocs(t *testing.T) {
	t.Run("average", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			dict := starlark.NewDict(st.N)
			for i := 0; i < st.N; i++ {
				key := starlark.MakeInt(i)
				dict.SetKey(key, starlark.None)
				if err := thread.AddAllocs(starlark.EstimateSize(key)); err != nil {
					st.Fatal(err)
				}
			}

			dict_values, _ := dict.Attr("values")
			if dict_values == nil {
				st.Fatal("no such method: dict.values")
			}

			result, err := starlark.Call(thread, dict_values, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("fixed", func(t *testing.T) {
		dict := starlark.NewDict(100)
		for i := 0; i < 100; i++ {
			key := starlark.MakeInt(i)
			dict.SetKey(key, starlark.None)
		}

		fn, _ := dict.Attr("values")
		if fn == nil {
			t.Fatal("no such method: dict.values")
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, fn, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestListAppendSteps(t *testing.T) {
}

func TestListAppendAllocs(t *testing.T) {
	list := starlark.NewList([]starlark.Value{})
	list_append, _ := list.Attr("append")
	if list_append == nil {
		t.Fatal("no such method: list.append")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, list_append, starlark.Tuple{starlark.None}, nil)
			if err != nil {
				st.Error(err)
			}
		}

		st.KeepAlive(list)
	})
}

func TestListClearSteps(t *testing.T) {
}

func TestListClearAllocs(t *testing.T) {
	const numTestElems = 10

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			list := starlark.NewList(make([]starlark.Value, 0, numTestElems))
			if err := thread.AddAllocs(starlark.EstimateSize(list)); err != nil {
				st.Error(err)
			}
			list_clear, _ := list.Attr("clear")
			if list_clear == nil {
				t.Fatal("no such method: list.clear")
			}

			for j := 0; j < numTestElems; j++ {
				list.Append(starlark.MakeInt(j))
			}

			_, err := starlark.Call(thread, list_clear, starlark.Tuple{}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(list)
		}
	})
}

func TestListExtendSteps(t *testing.T) {
}

func TestListExtendAllocs(t *testing.T) {
	const numTestElems = 10

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		list := starlark.NewList([]starlark.Value{})
		list_extend, _ := list.Attr("extend")
		if list_extend == nil {
			t.Fatal("no such method: list.extend")
		}

		iter := &unsafeTestIterable{t}
		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)
		_, err := starlark.Call(thread, list_extend, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("small-list", func(t *testing.T) {
		toAdd := starlark.NewList(make([]starlark.Value, 0, numTestElems))
		for i := 0; i < numTestElems; i++ {
			toAdd.Append(starlark.None)
		}

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			list := starlark.NewList([]starlark.Value{})
			list_extend, _ := list.Attr("extend")
			if list_extend == nil {
				st.Fatal("no such method: list.extend")
			}

			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, list_extend, starlark.Tuple{toAdd}, nil)
				if err != nil {
					st.Error(err)
				}
			}
			st.KeepAlive(list)
		})
	})

	t.Run("big-list", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			list := starlark.NewList([]starlark.Value{})
			list_extend, _ := list.Attr("extend")
			if list_extend == nil {
				t.Fatal("no such method: list.extend")
			}

			toAdd := starlark.NewList(make([]starlark.Value, st.N))
			for i := 0; i < st.N; i++ {
				toAdd.Append(starlark.None)
			}

			_, err := starlark.Call(thread, list_extend, starlark.Tuple{toAdd}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(list)
		})
	})

	t.Run("small-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
					resultSize := starlark.EstimateSize(&starlark.List{}) +
						starlark.EstimateMakeSize([]starlark.Value{}, 16)
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
					return starlark.NewList(make([]starlark.Value, 0, 16)), nil
				},
				maxN: 10,
			}
			list := starlark.NewList([]starlark.Value{})
			if err := thread.AddAllocs(starlark.EstimateSize(list)); err != nil {
				st.Error(err)
			}
			list_extend, _ := list.Attr("extend")
			if list_extend == nil {
				st.Fatal("no such method: list.extend")
			}

			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, list_extend, starlark.Tuple{iter}, nil)
				if err != nil {
					st.Error(err)
				}
			}

			st.KeepAlive(list)
		})
	})

	t.Run("big-iterable", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			list := starlark.NewList([]starlark.Value{})
			if err := thread.AddAllocs(starlark.EstimateSize(list)); err != nil {
				st.Error(err)
			}
			list_extend, _ := list.Attr("extend")
			if list_extend == nil {
				t.Fatal("no such method: list.extend")
			}
			iter := &testIterable{
				nth: func(thread *starlark.Thread, _ int) (starlark.Value, error) {
					resultSize := starlark.EstimateSize(&starlark.List{}) +
						starlark.EstimateMakeSize([]starlark.Value{}, 16)
					if err := thread.AddAllocs(resultSize); err != nil {
						return nil, err
					}
					return starlark.NewList(make([]starlark.Value, 0, 16)), nil
				},
				maxN: st.N,
			}

			_, err := starlark.Call(thread, list_extend, starlark.Tuple{iter}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(list)
		})
	})
}

func TestListIndexSteps(t *testing.T) {
}

func TestListIndexAllocs(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.None,
		starlark.False,
		starlark.True,
	})
	list_index, _ := list.Attr("index")
	if list_index == nil {
		t.Fatal("no such method: list.index")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			index, err := starlark.Call(thread, list_index, starlark.Tuple{starlark.False, starlark.None}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(index)
		}
	})
}

func TestListInsertSteps(t *testing.T) {
}

func TestListInsertAllocs(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		list := starlark.NewList([]starlark.Value{})
		list_insert, _ := list.Attr("insert")
		if list_insert == nil {
			st.Fatal("no such method: list.insert")
		}

		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, list_insert, starlark.Tuple{starlark.MakeInt(st.N / 2), starlark.None}, nil)
			if err != nil {
				st.Error(err)
			}
		}
		st.KeepAlive(list)
	})
}

func TestListPopSteps(t *testing.T) {
}

func TestListPopAllocs(t *testing.T) {
	const numTestElems = 10

	list := starlark.NewList(make([]starlark.Value, 0, numTestElems))
	list_pop, _ := list.Attr("pop")
	if list_pop == nil {
		t.Fatal("no such method: list.pop")
	}

	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			if list.Len() == 0 {
				for j := 0; j < numTestElems; j++ {
					list.Append(starlark.None)
				}
			}

			_, err := starlark.Call(thread, list_pop, starlark.Tuple{starlark.MakeInt(-1)}, nil)
			if err != nil {
				st.Error(err)
			}
		}

		st.KeepAlive(list)
	})
}

func TestListRemoveSteps(t *testing.T) {
}

func TestListRemoveAllocs(t *testing.T) {
	const numTestElems = 10
	preallocatedInts := make([]starlark.Value, numTestElems)
	list := starlark.NewList(make([]starlark.Value, 0, numTestElems))
	for i := 0; i < numTestElems; i++ {
		preallocatedInts[i] = starlark.MakeInt(i)
		list.Append(preallocatedInts[i])
	}

	list_remove, _ := list.Attr("remove")
	if list_remove == nil {
		t.Fatal("no such method: list.remove")
	}

	st := startest.From(t)
	st.SetMaxAllocs(0)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, list_remove, starlark.Tuple{starlark.MakeInt(i % numTestElems)}, nil)
			if err != nil {
				st.Error(err)
			}

			// re-add value for next iteration
			list.Append(preallocatedInts[i%numTestElems])
		}

		st.KeepAlive(list)
	})
}

func TestStringCapitalizeSteps(t *testing.T) {
}

func TestStringCapitalizeAllocs(t *testing.T) {
	string_capitalize, _ := starlark.String("ıııııııııı").Attr("capitalize")
	if string_capitalize == nil {
		t.Fatal("no such method: string.capitalize")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_capitalize, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func testStringIterable(t *testing.T, methodName string) {
	method, _ := starlark.String("arbitrary-string").Attr(methodName)
	if method == nil {
		t.Fatalf("no such method: string.%s", methodName)
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

func TestStringCodepointOrdsSteps(t *testing.T) {
}

func TestStringCodepointOrdsAllocs(t *testing.T) {
	testStringIterable(t, "codepoint_ords")
}

func TestStringCodepointsSteps(t *testing.T) {
}

func TestStringCodepointsAllocs(t *testing.T) {
	testStringIterable(t, "codepoints")
}

func TestStringCountSteps(t *testing.T) {
}

func TestStringCountAllocs(t *testing.T) {
	base := starlark.String(strings.Repeat("aab", 1000))
	arg := starlark.String("a")

	string_count, _ := base.Attr("count")
	if string_count == nil {
		t.Fatal("no such method: string.count")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_count, starlark.Tuple{arg}, nil)
			if err != nil {
				st.Error(err)
			}

			st.KeepAlive(result)
		}
	})
}

func TestStringElemOrdsSteps(t *testing.T) {
}

func TestStringElemOrdsAllocs(t *testing.T) {
	testStringIterable(t, "elem_ords")
}

func TestStringElemsSteps(t *testing.T) {
}

func TestStringElemsAllocs(t *testing.T) {
	testStringIterable(t, "elems")
}

func testStringFixSteps(t *testing.T) {
}

func testStringFixAllocs(t *testing.T, method_name string) {
	method, _ := starlark.String("foo-bar-foo").Attr(method_name)
	if method == nil {
		t.Fatalf("no such method: %s", method)
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.SetMaxAllocs(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			args := starlark.Tuple{starlark.String("foo")}
			result, err := starlark.Call(thread, method, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringEndswithSteps(t *testing.T) {
}

func TestStringEndswithAllocs(t *testing.T) {
	testStringFixAllocs(t, "endswith")
}

func testStringFindMethodSteps(t *testing.T) {
}

func testStringFindMethodAllocs(t *testing.T, name string) {
	haystack := starlark.String("Better safe than sorry")
	needle := starlark.String("safe")

	string_find, _ := haystack.Attr(name)
	if string_find == nil {
		t.Fatalf("no such method: string.%s", name)
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_find, starlark.Tuple{needle}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringFindSteps(t *testing.T) {
}

func TestStringFindAllocs(t *testing.T) {
	testStringFindMethodAllocs(t, "find")
}

func TestStringFormatSteps(t *testing.T) {
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

func TestStringIndexSteps(t *testing.T) {
}

func TestStringIndexAllocs(t *testing.T) {
	testStringFindMethodAllocs(t, "index")
}

func TestStringIsalnumSteps(t *testing.T) {
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

func TestStringIsalphaSteps(t *testing.T) {
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

func TestStringIsdigitSteps(t *testing.T) {
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

func TestStringIslowerSteps(t *testing.T) {
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

func TestStringIsspaceSteps(t *testing.T) {
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

func TestStringIstitleSteps(t *testing.T) {
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

func TestStringIsupperSteps(t *testing.T) {
}

func TestStringIsupperAllocs(t *testing.T) {
	string_istitle, _ := starlark.String("Hello, world!").Attr("isupper")
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

func TestStringJoinSteps(t *testing.T) {
}

func TestStringJoinAllocs(t *testing.T) {
	string_join, _ := starlark.String("aa").Attr("join")
	if string_join == nil {
		t.Fatal("no such method: string.join")
	}

	t.Run("safety-respected", func(t *testing.T) {
		const expected = "feature unavailable to the sandbox"

		thread := &starlark.Thread{}
		thread.RequireSafety(starlark.MemSafe)
		iter := &unsafeTestIterable{t}
		_, err := starlark.Call(thread, string_join, starlark.Tuple{iter}, nil)
		if err == nil {
			t.Error("expected error")
		} else if err.Error() != expected {
			t.Errorf("unexpected error: expected %v but got %v", expected, err)
		}
	})

	t.Run("growth", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			iter := &testIterable{
				maxN: st.N,
				nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
					return starlark.String("b"), nil
				},
			}
			args := starlark.Tuple{iter}
			result, err := starlark.Call(thread, string_join, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("result", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				iter := &testIterable{
					maxN: 10,
					nth: func(_ *starlark.Thread, _ int) (starlark.Value, error) {
						return starlark.String("b"), nil
					},
				}
				args := starlark.Tuple{iter}
				result, err := starlark.Call(thread, string_join, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringLowerSteps(t *testing.T) {
}

func TestStringLowerAllocs(t *testing.T) {
	t.Run("ASCII", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("dEaDbEeF", st.N))
			string_lower, _ := str.Attr("lower")
			if string_lower == nil {
				t.Fatalf("no such method: string.lower")
			}

			result, err := starlark.Call(thread, string_lower, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("ΔΗΑΔΒΗΗΦ", st.N))
			string_lower, _ := str.Attr("lower")
			if string_lower == nil {
				t.Fatalf("no such method: string.lower")
			}

			result, err := starlark.Call(thread, string_lower, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})

		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("a", st.N) + "ȺȾȺȾ")
			string_lower, _ := str.Attr("lower")
			if string_lower == nil {
				t.Fatalf("no such method: string.lower")
			}

			result, err := starlark.Call(thread, string_lower, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})

		st.RunThread(func(thread *starlark.Thread) {
			// This is the only case where the difference is 2
			// e.g. the lowercase version takes 1 byte, this
			// special char takes 3. However, since the computation
			// is done through the length of the original string,
			// it should be safe ("just" wasting a 2/3 of the space).
			str := starlark.String(strings.Repeat("K", st.N))
			string_lower, _ := str.Attr("lower")
			if string_lower == nil {
				t.Fatalf("no such method: string.lower")
			}

			result, err := starlark.Call(thread, string_lower, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode-single", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String("Φ")
			string_lower, _ := str.Attr("lower")
			if string_lower == nil {
				t.Fatalf("no such method: string.lower")
			}

			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_lower, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func testStringStripSteps(t *testing.T) {
}

func testStringStripAllocs(t *testing.T, method_name string) {
	method, _ := starlark.String("     ababaZZZZZababa     ").Attr(method_name)
	if method == nil {
		t.Fatalf("no such method: string.%s", method_name)
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

func TestStringLstripSteps(t *testing.T) {
}

func TestStringLstripAllocs(t *testing.T) {
	testStringStripAllocs(t, "lstrip")
}

func testStringPartitionMethodSteps(t *testing.T) {
}

func testStringPartitionMethodAllocs(t *testing.T, name string) {
	recv := starlark.String("don't communicate by sharing memory, share memory by communicating.")
	string_partition, _ := recv.Attr(name)
	if string_partition == nil {
		t.Fatalf("no such method: string.%s", name)
	}

	t.Run("not-present", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_partition, starlark.Tuple{starlark.String("channel")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("present", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_partition, starlark.Tuple{starlark.String("memory")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringPartitionSteps(t *testing.T) {
}

func TestStringPartitionAllocs(t *testing.T) {
	testStringPartitionMethodAllocs(t, "partition")
}

func testStringRemovefixSteps(t *testing.T) {
}

func testStringRemovefixAllocs(t *testing.T, method_name string) {
	method, _ := starlark.String("aaaaaZZZZZaaaaa").Attr(method_name)
	if method == nil {
		t.Fatalf("no such method: string.%s", method_name)
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			args := starlark.Tuple{starlark.String("aaaaa")}
			result, err := starlark.Call(thread, method, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringRemoveprefixSteps(t *testing.T) {
}

func TestStringRemoveprefixAllocs(t *testing.T) {
	testStringRemovefixAllocs(t, "removeprefix")
}

func TestStringRemovesuffixSteps(t *testing.T) {
}

func TestStringRemovesuffixAllocs(t *testing.T) {
	testStringRemovefixAllocs(t, "removesuffix")
}

func TestStringReplaceSteps(t *testing.T) {
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

func TestStringRfindSteps(t *testing.T) {
}

func TestStringRfindAllocs(t *testing.T) {
	testStringFindMethodAllocs(t, "rfind")
}

func TestStringRindexSteps(t *testing.T) {
}

func TestStringRindexAllocs(t *testing.T) {
	testStringFindMethodAllocs(t, "rindex")
}

func TestStringRpartitionSteps(t *testing.T) {
}

func TestStringRpartitionAllocs(t *testing.T) {
	testStringPartitionMethodAllocs(t, "rpartition")
}

func TestStringRsplitSteps(t *testing.T) {
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
			if err := thread.AddAllocs(starlark.EstimateMakeSize([]byte{}, len(str))); err != nil {
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
			if err := thread.AddAllocs(starlark.EstimateMakeSize([]byte{}, len(str))); err != nil {
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

func TestStringRstripSteps(t *testing.T) {
}

func TestStringRstripAllocs(t *testing.T) {
	testStringStripAllocs(t, "rstrip")
}

func TestStringSplitSteps(t *testing.T) {
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
			if err := thread.AddAllocs(starlark.EstimateMakeSize([]byte{}, len(str))); err != nil {
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
			if err := thread.AddAllocs(starlark.EstimateMakeSize([]byte{}, len(str))); err != nil {
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

func TestStringSplitlinesSteps(t *testing.T) {
}

func TestStringSplitlinesAllocs(t *testing.T) {
	str := starlark.String(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Suspendisse porta ipsum a purus pharetra sagittis.
Fusce tristique ex non fermentum suscipit.
Curabitur nec velit fringilla arcu lacinia commodo.`)

	string_splitlines, _ := str.Attr("splitlines")
	if string_splitlines == nil {
		t.Fatal("no such method: string.splitlines")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, string_splitlines, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestStringStartswithSteps(t *testing.T) {
}

func TestStringStartswithAllocs(t *testing.T) {
	testStringFixAllocs(t, "startswith")
}

func TestStringStripSteps(t *testing.T) {
}

func TestStringStripAllocs(t *testing.T) {
	testStringStripAllocs(t, "strip")
}

func TestStringTitleSteps(t *testing.T) {
}

func TestStringTitleAllocs(t *testing.T) {
	t.Run("ASCII", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("dEaDbEeF", st.N))
			string_title, _ := str.Attr("title")
			if string_title == nil {
				t.Fatal("no such method: string.title")
			}

			result, err := starlark.Call(thread, string_title, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)

		// Same byte-length capitals
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("δηαδβηηφ", st.N))
			string_title, _ := str.Attr("title")
			if string_title == nil {
				t.Fatal("no such method: string.title")
			}

			result, err := starlark.Call(thread, string_title, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})

		// Different byte-length capitals
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("ı ", st.N))
			string_title, _ := str.Attr("title")
			if string_title == nil {
				t.Fatal("no such method: string.title")
			}

			result, err := starlark.Call(thread, string_title, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode-single", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String("φ")
			string_title, _ := str.Attr("title")
			if string_title == nil {
				t.Fatalf("no such method: string.title")
			}

			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_title, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringUpperSteps(t *testing.T) {
}

func TestStringUpperAllocs(t *testing.T) {
	t.Run("ASCII", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("dEaDbEeF", st.N))
			string_upper, _ := str.Attr("upper")
			if string_upper == nil {
				t.Fatalf("no such method: string.upper")
			}

			result, err := starlark.Call(thread, string_upper, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)

		// In this case, the characters are not ascii, but the length
		// of each character remains stable for each character.
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("δηαδβηηφ", st.N))
			string_upper, _ := str.Attr("upper")
			if string_upper == nil {
				t.Fatalf("no such method: string.upper")
			}

			result, err := starlark.Call(thread, string_upper, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})

		// In this case, the characters at the end of the string
		// have a bigger representations after conversion thus
		// triggering a growth operation which is rather badly
		// implemented leading to an allocation which is double
		// the size it actually needs. In go 1.20, while the
		// growth operation is still flawed, it will not be used
		// during rune encoding, thus reducing the size of this
		// allocation to about 20% (except for small strings, where
		// the problem persists)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("a", st.N) + "ɥɐɥɐ")
			string_upper, _ := str.Attr("upper")
			if string_upper == nil {
				t.Fatalf("no such method: string.upper")
			}

			result, err := starlark.Call(thread, string_upper, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})

		// In this case, the size of the translated rule is
		// smaller than the original one. But even so, go
		// will proceed to allocate a very big buffer, about
		// twice the size of the original string.
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String(strings.Repeat("ı", st.N))
			string_upper, _ := str.Attr("upper")
			if string_upper == nil {
				t.Fatalf("no such method: string.upper")
			}

			result, err := starlark.Call(thread, string_upper, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})

	t.Run("Unicode-single", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			str := starlark.String("φ")
			string_upper, _ := str.Attr("upper")
			if string_upper == nil {
				t.Fatalf("no such method: string.upper")
			}

			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_upper, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestSetUnionSteps(t *testing.T) {
}

func TestSetUnionAllocs(t *testing.T) {
	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		set := starlark.NewSet(st.N / 2)
		for i := 0; i < st.N/2; i++ {
			n := starlark.Value(starlark.MakeInt(i))
			if err := thread.AddAllocs(starlark.EstimateSize(n)); err != nil {
				st.Error(err)
			}
			set.Insert(n)
		}

		set_union, _ := set.Attr("union")
		if set_union == nil {
			t.Fatal("no such method: set.union")
		}

		iter := testIterable{
			maxN: st.N,
			nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
				result := starlark.MakeInt(n)
				if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
					return nil, err
				}
				return result, nil
			},
		}
		result, err := starlark.Call(thread, set_union, starlark.Tuple{&iter}, nil)
		if err != nil {
			st.Error(err)
		}
		st.KeepAlive(result)
	})
}

func TestSafeIterateSteps(t *testing.T) {
}

func TestSafeIterateAllocs(t *testing.T) {
	t.Run("non-allocating", func(t *testing.T) {
		st := startest.From(t)
		st.SetMaxAllocs(0)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			nonAllocating := &testIterable{
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					return starlark.True, nil
				},
			}
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
			allocating := &testIterable{
				nth: func(thread *starlark.Thread, n int) (starlark.Value, error) {
					tupleSize := starlark.EstimateMakeSize(starlark.Tuple{}, 16) + starlark.SliceTypeOverhead
					if err := thread.AddAllocs(tupleSize); err != nil {
						return nil, err
					}
					return make(starlark.Tuple, 16), nil
				},
			}
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
