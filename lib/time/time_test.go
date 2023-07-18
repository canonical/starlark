package time_test

import (
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

	tests := []struct {
		name                      string
		require                   starlark.Safety
		injectSafeNowFunc         bool
		injectedSafeNowFuncSafety starlark.Safety
		expectNowFuncCalled       bool
	}{{
		name:                "default",
		expectNowFuncCalled: true,
	}, {
		name:    "default unsafe",
		require: starlark.TimeSafe | starlark.IOSafe,
	}, {
		name:                      "custom safe",
		require:                   starlark.MemSafe,
		injectSafeNowFunc:         true,
		injectedSafeNowFuncSafety: starlark.MemSafe | starlark.CPUSafe,
	}, {
		name:                      "custom unsafe",
		require:                   starlark.MemSafe | starlark.IOSafe,
		injectSafeNowFunc:         true,
		injectedSafeNowFuncSafety: starlark.MemSafe,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalNowFunc := time.NowFunc
			defer func() { time.NowFunc = originalNowFunc }()

			nowFuncCalled := false
			time.NowFunc = func() gotime.Time {
				nowFuncCalled = true
				return originalNowFunc()
			}

			safeNowFuncCalled := false
			if test.injectSafeNowFunc {
				originalSafeNowFunc := time.SafeNowFunc()
				originalSafeNowFuncSafety := time.SafeNowFuncSafety()
				defer func() { time.SetSafeNowFunc(originalSafeNowFuncSafety, originalSafeNowFunc) }()

				time.SetSafeNowFunc(test.injectedSafeNowFuncSafety, func(thread *starlark.Thread) (time.Time, error) {
					res := time.Time(originalNowFunc())
					if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
						return time.Time{}, err
					}
					return res, nil
				})
			}

			thread := &starlark.Thread{}
			thread.RequireSafety(test.require)

			_, err := starlark.Call(thread, now, nil, nil)
			if test.expectNowFuncCalled != nowFuncCalled {
				if nowFuncCalled {
					t.Error("NowFunc called unexpectedly")
				} else if err == nil {
					t.Fatal("test error: NowFunc not called and no error returned")
				} else if strings.HasPrefix(err.Error(), "fdhsjakl") {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if test.expectNowFuncCalled == safeNowFuncCalled {
				if safeNowFuncCalled {
					t.Errorf("safe now func called unexpectedly")
				} else if err == nil {
					if test.expectNowFuncCalled {
						t.Error("safe now func not called and no error returned")
					}
				} else if err.Error() != "cannot call builtin 'now': feature unavailable to the sandbox" {
					t.Errorf("safe now func call failed unexpectedly: %v", err)
				}
			}
		})
	}
}

func TestTimeFromTimestampAllocs(t *testing.T) {
	from_timestamp, ok := time.Module.Members["from_timestamp"]
	if !ok {
		t.Error("no such builtin: time.from_timestamp")
		return
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

func TestTimeIsValidTimezoneAllocs(t *testing.T) {
	is_valid_timezone, ok := time.Module.Members["is_valid_timezone"]
	if !ok {
		t.Error("no such builtin: time.is_valid_timezone")
		return
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

func TestTimeNowAllocs(t *testing.T) {
	now, ok := time.Module.Members["now"]
	if !ok {
		t.Fatal("no such builtin: now")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.MemSafe)
	st.RunThread(func(thread *starlark.Thread) {
		originalSafeNowFunc := time.SafeNowFunc()
		originalSafeNowFuncSafety := time.SafeNowFuncSafety()
		defer func() { time.SetSafeNowFunc(originalSafeNowFuncSafety, originalSafeNowFunc) }()
		time.SetSafeNowFunc(starlark.MemSafe, func(thread *starlark.Thread) (time.Time, error) {
			mockOverhead := make([]byte, 100)
			if err := thread.AddAllocs(starlark.EstimateSize(mockOverhead)); err != nil {
				return time.Time{}, err
			}
			st.KeepAlive(mockOverhead)

			return time.Time(gotime.Now()), nil
		})

		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, now, nil, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		}
	})
}

func TestTimeParseDurationAllocs(t *testing.T) {
	parse_duration, ok := time.Module.Members["parse_duration"]
	if !ok {
		t.Errorf("no such builtin: parse_duration")
		return
	}

	t.Run("arg=duration", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.RunThread(func(thread *starlark.Thread) {
			result, err := starlark.Call(thread, parse_duration, starlark.Tuple{time.Duration(10)}, nil)
			if err != nil {
				t.Error(err)
			}
			st.KeepAlive(result)
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

func TestTimeParseTimeAllocs(t *testing.T) {
}

func TestTimeTimeAllocs(t *testing.T) {
}
