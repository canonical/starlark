package time_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	gotime "time"

	"github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range time.Module.Members {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := time.Safeties[name]; !ok {
			t.Errorf("builtin time.%s has no safety declaration", name)
		} else if actualSafety := builtin.Safety(); actualSafety != safety {
			t.Errorf("builtin time.%s has incorrect safety: expected %v but got %v", name, safety, actualSafety)
		}
	}
	for name, _ := range time.Safeties {
		if _, ok := time.Module.Members[name]; !ok {
			t.Errorf("no method for safety declaration time.%s", name)
		}
	}
}

func TestMethodSafetiesExist(t *testing.T) {
	for name, _ := range time.TimeMethods {
		if _, ok := time.TimeMethodSafeties[name]; !ok {
			t.Errorf("builtin time.%s has no safety declaration", name)
		}
	}
	for name, _ := range time.TimeMethodSafeties {
		if _, ok := time.TimeMethods[name]; !ok {
			t.Errorf("no method for safety declaration time.%s", name)
		}
	}
}

func TestTimeNowSafety(t *testing.T) {
	now, ok := time.Module.Members["now"]
	if !ok {
		t.Fatal("no such builtin: now")
	}

	nowSafety, ok := time.Safeties["now"]
	if !ok {
		t.Fatal("no safety for builtin: now")
	}
	if nowSafety == starlark.NotSafe {
		t.Fatal("now builtin is not safe")
	}

	safeThreadSafety := nowSafety
	safeThread := &starlark.Thread{}
	safeThread.RequireSafety(safeThreadSafety)

	tests := []struct {
		name          string
		thread        *starlark.Thread
		nowFuncSafety starlark.SafetyFlags
		expectError   bool
	}{{
		name:          "default",
		thread:        &starlark.Thread{},
		nowFuncSafety: time.NowFuncSafety,
	}, {
		name:          "no-safety-required",
		thread:        &starlark.Thread{},
		nowFuncSafety: starlark.NotSafe,
	}, {
		name:          "not-safe",
		thread:        safeThread,
		nowFuncSafety: starlark.NotSafe,
		expectError:   true,
	}, {
		name:          "safe",
		thread:        safeThread,
		nowFuncSafety: safeThreadSafety,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalNowFuncSafety := time.NowFuncSafety
			time.NowFuncSafety = test.nowFuncSafety
			defer func() { time.NowFuncSafety = originalNowFuncSafety }()

			_, err := starlark.Call(test.thread, now, nil, nil)
			if err == nil {
				if test.expectError {
					t.Error("expected error")
				}
			} else {
				expected := &starlark.SafetyFlagsError{}
				if !test.expectError || !errors.As(err, &expected) {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestTimeFromTimestampSteps(t *testing.T) {
	from_timestamp, ok := time.Module.Members["from_timestamp"]
	if !ok {
		t.Fatalf("no such builtin: time.from_timestamp")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.SetMaxExecutionSteps(0)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, from_timestamp, starlark.Tuple{starlark.MakeInt(10000)}, nil)
			if err != nil {
				st.Error(err)
			}
		}
	})
}

func TestTimeFromTimestampAllocs(t *testing.T) {
	from_timestamp, ok := time.Module.Members["from_timestamp"]
	if !ok {
		t.Fatalf("no such builtin: time.from_timestamp")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, from_timestamp, starlark.Tuple{starlark.MakeInt(10000)}, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestTimeIsValidTimezoneSteps(t *testing.T) {
}

func TestTimeIsValidTimezoneAllocs(t *testing.T) {
	is_valid_timezone, ok := time.Module.Members["is_valid_timezone"]
	if !ok {
		t.Fatalf("no such builtin: time.is_valid_timezone")
	}

	t.Run("timezone=valid", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, is_valid_timezone, starlark.Tuple{starlark.String("Europe/Prague")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("timezone=invalid", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, is_valid_timezone, starlark.Tuple{starlark.String("Middle_Earth/Minas_Tirith")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestTimeNowSteps(t *testing.T) {
	now, ok := time.Module.Members["now"]
	if !ok {
		t.Fatal("no such builtin: now")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			_, err := starlark.Call(thread, now, nil, nil)
			if err != nil {
				st.Error(err)
			}
		}
	})
}

func TestTimeNowAllocs(t *testing.T) {
	now, ok := time.Module.Members["now"]
	if !ok {
		t.Fatal("no such builtin: now")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, now, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestTimeParseDurationSteps(t *testing.T) {
	parse_duration, ok := time.Module.Members["parse_duration"]
	if !ok {
		t.Fatalf("no such builtin: parse_duration")
	}

	t.Run("arg=duration", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxExecutionSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_duration, starlark.Tuple{time.Duration(10)}, nil)
				if err != nil {
					t.Error(err)
				}
			}
		})
	})

	t.Run("arg=string", func(t *testing.T) {
		const timestamp = "10h47m"

		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMinExecutionSteps(uint64(len(timestamp)))
		st.SetMaxExecutionSteps(uint64(len(timestamp)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_duration, starlark.Tuple{starlark.String(timestamp)}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})
}

func TestTimeParseDurationAllocs(t *testing.T) {
	parse_duration, ok := time.Module.Members["parse_duration"]
	if !ok {
		t.Fatalf("no such builtin: parse_duration")
	}

	t.Run("arg=duration", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_duration, starlark.Tuple{time.Duration(10)}, nil)
				if err != nil {
					t.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("arg=string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_duration, starlark.Tuple{starlark.String("10h47m")}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestTimeParseTimeSteps(t *testing.T) {
}

func TestTimeParseTimeAllocs(t *testing.T) {
	parse_time, ok := time.Module.Members["parse_time"]
	if !ok {
		t.Fatalf("no such builtin: parse_time")
	}

	t.Run("default-args", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			raw := starlark.String("2011-11-11T12:00:00Z")
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_time, starlark.Tuple{raw}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-format", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			raw := starlark.String("2011-11-11")
			format := starlark.String("2006-01-02")
			args := starlark.Tuple{raw, format}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_time, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-location", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			raw := starlark.String("2011-11-11")
			format := starlark.String("2006-01-02")
			location := starlark.String("Europe/Riga")
			args := starlark.Tuple{raw, format, location}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_time, args, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestTimeTimeSteps(t *testing.T) {
}

func TestTimeTimeAllocs(t *testing.T) {
}

func TestSafeDurationUnpacker(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
		st.SetMaxAllocs(0)
		st.SetMaxExecutionSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				d := time.Duration(10)

				sdu := time.SafeDurationUnpacker{}
				sdu.BindThread(thread)
				if err := starlark.UnpackPositionalArgs("parse_duration", starlark.Tuple{d}, nil, 1, &sdu); err != nil {
					st.Error(err)
				}

				result := sdu.Duration()
				if result != d {
					st.Errorf("incorrect value returned: expected %v but got %v", d, result)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("string", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe | starlark.CPUSafe)
		st.SetMaxAllocs(0)
		st.SetMinExecutionSteps(uint64(len("1h")))
		st.SetMaxExecutionSteps(uint64(len("1h")))
		st.RunThread(func(thread *starlark.Thread) {
			expected, err := gotime.ParseDuration(fmt.Sprintf("%dh", st.N))
			if err != nil {
				st.Fatal(err)
			}
			expectedDuration := time.Duration(expected)

			raw := starlark.String(strings.Repeat("1h", st.N))
			sdu := time.SafeDurationUnpacker{}
			sdu.BindThread(thread)
			if err := starlark.UnpackPositionalArgs("parse_duration", starlark.Tuple{raw}, nil, 1, &sdu); err != nil {
				st.Error(err)
			}

			result := sdu.Duration()
			if result != expectedDuration {
				st.Errorf("incorrect value returned: expected %v but got %v", expectedDuration, result)
			}
			st.KeepAlive(result)
		})
	})
}

func TestSafeString(t *testing.T) {
	tests := []struct {
		name  string
		input starlark.SafeStringer
	}{{
		name:  "Duration",
		input: time.Duration(gotime.Second),
	}, {
		name:  "Time",
		input: time.Time(gotime.Now()),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("nil-thread", func(t *testing.T) {
				builder := new(strings.Builder)
				if err := test.input.SafeString(nil, builder); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})

			t.Run("consistency", func(t *testing.T) {
				thread := &starlark.Thread{}
				builder := new(strings.Builder)
				if err := test.input.SafeString(thread, builder); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if stringer, ok := test.input.(fmt.Stringer); ok {
					expected := stringer.String()
					actual := builder.String()
					if expected != actual {
						t.Errorf("inconsistent stringer implementation: expected %s got %s", expected, actual)
					}
				}
			})
		})
	}
}
