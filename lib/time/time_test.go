package time_test

import (
	"testing"

	"github.com/canonical/starlark/lib/time"
)

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
