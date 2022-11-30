package time_test

import (
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

func TestTimeFromTimestampAllocs(t *testing.T) {
}

func TestTimeIsValidTimezoneAllocs(t *testing.T) {
}

func TestTimeNowAllocs(t *testing.T) {
}

func TestTimeParseDurationAllocs(t *testing.T) {
	parse_duration, ok := time.Module.Members["parse_duration"]
	if !ok {
		t.Errorf("No such builtin: parse_duration")
		return
	}

	t.Run("arg=duration", func(t *testing.T) {
		st := startest.From(t)
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
