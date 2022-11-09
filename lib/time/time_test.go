package time_test

import (
	"testing"

	"github.com/canonical/starlark/lib/time"
	"github.com/canonical/starlark/starlark"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range time.Module.Members {
		if builtin, ok := value.(*starlark.Builtin); ok {
			if safety, ok := time.Safeties[name]; !ok {
				t.Errorf("method %s has no safety declaration", name)
			} else if actualSafety := builtin.Safety(); actualSafety != safety {
				t.Errorf("builtin %s has incorrect safety: expected %v but got %v", name, safety, actualSafety)
			}
		}
	}
	for name, _ := range time.Safeties {
		if _, ok := time.Module.Members[name]; !ok {
			t.Errorf("no method for safety declaration %s", name)
		}
	}
}

func TestMethodSafetiesExist(t *testing.T) {
	for name, _ := range time.TimeMethods {
		if _, ok := time.TimeMethodSafeties[name]; !ok {
			t.Errorf("method %s has no safety declaration", name)
		}
	}
	for name, _ := range time.TimeMethodSafeties {
		if _, ok := time.TimeMethods[name]; !ok {
			t.Errorf("no method for safety declaration %s", name)
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
}

func TestTimeParseTimeAllocs(t *testing.T) {
}

func TestTimeTimeAllocs(t *testing.T) {
}
