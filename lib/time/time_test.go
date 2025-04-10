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

func isStarlarkCancellation(err error) bool {
	return strings.Contains(err.Error(), "Starlark computation cancelled:")
}

func TestPerThreadNowReturnsCorrectTime(t *testing.T) {
	th := &starlark.Thread{}
	date := gotime.Date(1, 2, 3, 4, 5, 6, 7, gotime.UTC)
	time.SetNow(th, func() (gotime.Time, error) {
		return date, nil
	})

	res, err := starlark.Call(th, time.Module.Members["now"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	retTime := gotime.Time(res.(time.Time))

	if !retTime.Equal(date) {
		t.Fatal("Expected time to be equal", retTime, date)
	}
}

func TestPerThreadNowReturnsError(t *testing.T) {
	th := &starlark.Thread{}
	e := errors.New("no time")
	time.SetNow(th, func() (gotime.Time, error) {
		return gotime.Time{}, e
	})

	_, err := starlark.Call(th, time.Module.Members["now"], nil, nil)
	if !errors.Is(err, e) {
		t.Fatal("Expected equal error", e, err)
	}
}

func TestGlobalNowReturnsCorrectTime(t *testing.T) {
	th := &starlark.Thread{}

	oldNow := time.NowFunc
	defer func() {
		time.NowFunc = oldNow
	}()

	date := gotime.Date(1, 2, 3, 4, 5, 6, 7, gotime.UTC)
	time.NowFunc = func() gotime.Time {
		return date
	}

	res, err := starlark.Call(th, time.Module.Members["now"], nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	retTime := gotime.Time(res.(time.Time))

	if !retTime.Equal(date) {
		t.Fatal("Expected time to be equal", retTime, date)
	}
}

func TestGlobalNowReturnsErrorWhenNil(t *testing.T) {
	th := &starlark.Thread{}

	oldNow := time.NowFunc
	defer func() {
		time.NowFunc = oldNow
	}()

	time.NowFunc = nil

	_, err := starlark.Call(th, time.Module.Members["now"], nil, nil)
	if err == nil {
		t.Fatal("Expected to get an error")
	}
}

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

func TestTimeFromTimestampSteps(t *testing.T) {
	from_timestamp, ok := time.Module.Members["from_timestamp"]
	if !ok {
		t.Fatalf("no such builtin: time.from_timestamp")
	}

	st := startest.From(t)
	st.RequireSafety(starlark.CPUSafe)
	st.SetMaxSteps(0)
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
	is_valid_timezone, ok := time.Module.Members["is_valid_timezone"]
	if !ok {
		t.Fatalf("no such builtin: time.is_valid_timezone")
	}

	t.Run("timezone=valid", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, is_valid_timezone, starlark.Tuple{starlark.String("Europe/Prague")}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})

	t.Run("timezone=invalid", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMaxSteps(0)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, is_valid_timezone, starlark.Tuple{starlark.String("Middle_Earth/Minas_Tirith")}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})
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
		st.SetMaxSteps(0)
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
		st.SetMinSteps(int64(len(timestamp)))
		st.SetMaxSteps(int64(len(timestamp)))
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
	parse_time, ok := time.Module.Members["parse_time"]
	if !ok {
		t.Fatalf("no such builtin: parse_time")
	}

	t.Run("default-args", func(t *testing.T) {
		date := starlark.String("2011-11-11T12:00:00Z")

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(date)))
		st.SetMinSteps(int64(len(date)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_time, starlark.Tuple{date}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})

	t.Run("with-format", func(t *testing.T) {
		date := starlark.String("2011-11-11")
		format := starlark.String("2006-01-02")

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(date)))
		st.SetMinSteps(int64(len(date)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_time, starlark.Tuple{date, format}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})

	t.Run("with-location", func(t *testing.T) {
		date := starlark.String("2011-11-11")
		format := starlark.String("2006-01-02")
		location := starlark.String("Europe/Riga")

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(date)))
		st.SetMinSteps(int64(len(date)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_time, starlark.Tuple{date, format, location}, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})

	t.Run("malformed-date-too-long", func(t *testing.T) {
		date := starlark.String("2011-2011")
		format := starlark.String("2006")

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(format)))
		st.SetMaxSteps(int64(len(format)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_time, starlark.Tuple{date, format}, nil)
				if err == nil {
					st.Error("error expected")
				} else if err.Error() != `parsing time "2011-2011": extra text: "-2011"` {
					st.Error(err)
				}
			}
		})
	})

	t.Run("malformed-date-too-short", func(t *testing.T) {
		date := starlark.String("2011")
		format := starlark.String("2006-01-02")

		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(date)))
		st.SetMaxSteps(int64(len(date)))
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, parse_time, starlark.Tuple{date, format}, nil)
				if err == nil {
					st.Error("error expected")
				} else if err.Error() != `parsing time "2011" as "2006-01-02": cannot parse "" as "-"` {
					st.Error(err)
				}
			}
		})
	})
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
			date := starlark.String("2011-11-11T12:00:00Z")
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, parse_time, starlark.Tuple{date}, nil)
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
			date := starlark.String("2011-11-11")
			format := starlark.String("2006-01-02")
			args := starlark.Tuple{date, format}
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
			date := starlark.String("2011-11-11")
			format := starlark.String("2006-01-02")
			location := starlark.String("Europe/Riga")
			args := starlark.Tuple{date, format, location}
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
	time_, ok := time.Module.Members["time"]
	if !ok {
		t.Fatal("no such builtin: time.time")
	}

	tests := []struct {
		kwarg string
		value starlark.Value
	}{{
		kwarg: "year",
		value: starlark.MakeInt(2011),
	}, {
		kwarg: "month",
		value: starlark.MakeInt(11),
	}, {
		kwarg: "day",
		value: starlark.MakeInt(11),
	}, {
		kwarg: "minute",
		value: starlark.MakeInt(11),
	}, {
		kwarg: "second",
		value: starlark.MakeInt(11),
	}, {
		kwarg: "nanosecond",
		value: starlark.MakeInt(11),
	}, {
		kwarg: "location",
		value: starlark.String("Europe/Riga"),
	}}
	for _, test := range tests {
		t.Run(test.kwarg, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMaxSteps(0)
			st.RunThread(func(thread *starlark.Thread) {
				kwargs := []starlark.Tuple{
					{starlark.String(test.kwarg), test.value},
				}
				for i := 0; i < st.N; i++ {
					_, err := starlark.Call(thread, time_, nil, kwargs)
					if err != nil {
						st.Error(err)
					}
				}
			})
		})
	}
}

func TestTimeTimeAllocs(t *testing.T) {
	time_, ok := time.Module.Members["time"]
	if !ok {
		t.Fatal("no such builtin: time.time")
	}

	tests := []struct {
		name, kwarg string
		value       starlark.Value
	}{{
		name:  "year",
		kwarg: "year",
		value: starlark.MakeInt(2011),
	}, {
		name:  "month",
		kwarg: "month",
		value: starlark.MakeInt(11),
	}, {
		name:  "day",
		kwarg: "day",
		value: starlark.MakeInt(11),
	}, {
		name:  "minute",
		kwarg: "minute",
		value: starlark.MakeInt(11),
	}, {
		name:  "second",
		kwarg: "second",
		value: starlark.MakeInt(11),
	}, {
		name:  "nanosecond",
		kwarg: "nanosecond",
		value: starlark.MakeInt(11),
	}, {
		name:  "location (UTC)",
		kwarg: "location",
		value: starlark.String("UTC"),
	}, {
		name:  "location (Local)",
		kwarg: "location",
		value: starlark.String("Local"),
	}, {
		name:  "location (Other)",
		kwarg: "location",
		value: starlark.String("Europe/Riga"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				kwargs := []starlark.Tuple{
					{starlark.String(test.kwarg), test.value},
				}
				for i := 0; i < st.N; i++ {
					result, err := starlark.Call(thread, time_, nil, kwargs)
					if err != nil {
						st.Error(err)
					}
					st.KeepAlive(result)
				}
			})
		})
	}
}

func TestTimeInLocationSteps(t *testing.T) {
	time := time.Time(gotime.Now())
	time_in_location, _ := time.Attr("in_location")
	if time_in_location == nil {
		t.Fatal("no such method: time.in_location")
	}

	tests := []struct {
		name, input string
	}{{
		name:  "UTC",
		input: "UTC",
	}, {
		name:  "Local",
		input: "Local",
	}, {
		name:  "Other",
		input: "Europe/Riga",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.CPUSafe)
			st.SetMaxSteps(0)
			st.RunThread(func(thread *starlark.Thread) {
				args := starlark.Tuple{starlark.String(test.input)}
				for i := 0; i < st.N; i++ {
					_, err := starlark.Call(thread, time_in_location, args, nil)
					if err != nil {
						st.Error(err)
					}
				}
			})
		})
	}
}

func TestTimeInLocationAllocs(t *testing.T) {
	time_ := time.Time(gotime.Now())
	time_in_location, _ := time_.Attr("in_location")
	if time_in_location == nil {
		t.Fatal("no such method: time.in_location")
	}

	tests := []struct {
		name, input string
	}{{
		name:  "UTC",
		input: "UTC",
	}, {
		name:  "Local",
		input: "Local",
	}, {
		name:  "Other",
		input: "Europe/Riga",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			st := startest.From(t)
			st.RequireSafety(starlark.MemSafe)
			st.RunThread(func(thread *starlark.Thread) {
				args := starlark.Tuple{starlark.String(test.input)}
				for i := 0; i < st.N; i++ {
					result, err := starlark.Call(thread, time_in_location, args, nil)
					if err != nil {
						st.Error(err)
					}
					st.KeepAlive(result)
				}
			})
		})
	}
}

func TestTimeFormatSteps(t *testing.T) {
	format := fmt.Sprintf("(%s)", gotime.Layout)
	time_ := time.Time(gotime.Unix(0, 0))
	time_format, _ := time_.Attr("format")
	if time_format == nil {
		t.Fatal("no such method: time.format")
	}

	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(format)))
		st.SetMaxSteps(int64(len(format)))
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String(format)}
			for i := 0; i < st.N; i++ {
				_, err := starlark.Call(thread, time_format, args, nil)
				if err != nil {
					st.Error(err)
				}
			}
		})
	})

	t.Run("big", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe)
		st.SetMinSteps(int64(len(format)))
		st.SetMaxSteps(int64(len(format)))
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String(strings.Repeat(format, st.N))}
			_, err := starlark.Call(thread, time_format, args, nil)
			if err != nil {
				st.Error(err)
			}
		})
	})
}

func TestTimeFormatAllocs(t *testing.T) {
	format := fmt.Sprintf("(%s)", gotime.Layout)
	time_ := time.Time(gotime.Unix(0, 0))
	time_format, _ := time_.Attr("format")
	if time_format == nil {
		t.Fatal("no such method: time.format")
	}

	t.Run("small", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String(format)}
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, time_format, args, nil)
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
		st.RunThread(func(thread *starlark.Thread) {
			args := starlark.Tuple{starlark.String(strings.Repeat(gotime.Layout, st.N))}
			result, err := starlark.Call(thread, time_format, args, nil)
			if err != nil {
				st.Error(err)
			}
			st.KeepAlive(result)
		})
	})
}

func TestSafeDurationUnpacker(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.SetMaxSteps(0)
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
		st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
		st.SetMaxAllocs(0)
		st.SetMinSteps(int64(len("1h")))
		st.SetMaxSteps(int64(len("1h")))
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

			t.Run("cancellation", func(t *testing.T) {
				st := startest.From(t)
				st.RequireSafety(starlark.TimeSafe)
				st.SetMaxSteps(0)
				st.RunThread(func(thread *starlark.Thread) {
					thread.Cancel("done")
					builder := starlark.NewSafeStringBuilder(thread)
					err := test.input.SafeString(thread, builder)
					if err == nil {
						st.Error("expected cancellation")
					} else if !isStarlarkCancellation(err) {
						st.Errorf("expected cancellation, got: %v", err)
					}
				})
			})
		})
	}
}

func TestSafeAttr(t *testing.T) {
	runTest := func(t *testing.T, input starlark.HasSafeAttrs) {
		for _, attr := range input.AttrNames() {
			t.Run(attr, func(t *testing.T) {
				t.Run("nil-thread", func(t *testing.T) {
					_, err := input.SafeAttr(nil, attr)
					if err != nil {
						t.Errorf("unexpected error: %v", err)
					}
				})

				t.Run("resources", func(t *testing.T) {
					st := startest.From(t)
					st.RequireSafety(starlark.CPUSafe | starlark.MemSafe)
					st.SetMaxSteps(0)
					st.RunThread(func(thread *starlark.Thread) {
						for i := 0; i < st.N; i++ {
							result, err := input.SafeAttr(thread, attr)
							if err != nil {
								st.Error(err)
							}
							st.KeepAlive(result)
						}
					})
				})

				t.Run("cancellation", func(t *testing.T) {
					st := startest.From(t)
					st.RequireSafety(starlark.TimeSafe)
					st.SetMaxSteps(0)
					st.RunThread(func(thread *starlark.Thread) {
						thread.Cancel("done")
						_, err := input.SafeAttr(thread, attr)
						if err != nil {
							st.Error(err)
						}
					})
				})
			})
		}
	}

	t.Run("Duration", func(t *testing.T) {
		runTest(t, time.Duration(gotime.Second))
	})

	t.Run("Time", func(t *testing.T) {
		runTest(t, time.Time(gotime.Now()))
	})
}
