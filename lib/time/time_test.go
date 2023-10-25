package time_test

import (
	"errors"
	"testing"

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
		expect        error
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
		expect:        starlark.ErrSafety,
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
			if err != nil {
				if test.expect == nil {
					t.Errorf("unexpected error: %v", err)
				} else if !errors.Is(err, test.expect) {
					t.Errorf("unexpected error: expected %t but got %t", test.expect, err)
				}
			} else if test.expect != nil {
				t.Errorf("now returned no error, expected: %v", test.expect)
			}
		})
	}
}

func TestTimeFromTimestampSteps(t *testing.T) {
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
}

func TestTimeTimeSteps(t *testing.T) {
}

func TestTimeTimeAllocs(t *testing.T) {
}
