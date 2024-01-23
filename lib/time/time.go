// Copyright 2021 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package time provides time-related constants and functions.
package time // import "github.com/canonical/starlark/lib/time"

import (
	"fmt"
	"sort"
	"time"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/starlarkstruct"
	"github.com/canonical/starlark/syntax"
)

// Module time is a Starlark module of time-related functions and constants.
// The module defines the following functions:
//
//	    from_timestamp(sec, nsec) - Converts the given Unix time corresponding to the number of seconds
//	                                and (optionally) nanoseconds since January 1, 1970 UTC into an object
//	                                of type Time. For more details, refer to https://pkg.go.dev/time#Unix.
//
//	    is_valid_timezone(loc) - Reports whether loc is a valid time zone name.
//
//	    now() - Returns the current local time. Applications may replace this function by a deterministic one.
//
//	    parse_duration(d) - Parses the given duration string. For more details, refer to
//	                        https://pkg.go.dev/time#ParseDuration.
//
//	    parse_time(x, format, location) - Parses the given time string using a specific time format and location.
//	                                     The expected arguments are a time string (mandatory), a time format
//	                                     (optional, set to RFC3339 by default, e.g. "2021-03-22T23:20:50.52Z")
//	                                     and a name of location (optional, set to UTC by default). For more details,
//	                                     refer to https://pkg.go.dev/time#Parse and https://pkg.go.dev/time#ParseInLocation.
//
//	    time(year, month, day, hour, minute, second, nanosecond, location) - Returns the Time corresponding to
//		                                                                        yyyy-mm-dd hh:mm:ss + nsec nanoseconds
//	                                                                         in the appropriate zone for that time
//	                                                                         in the given location. All the parameters
//	                                                                         are optional.
//
// The module also defines the following constants:
//
//	nanosecond - A duration representing one nanosecond.
//	microsecond - A duration representing one microsecond.
//	millisecond - A duration representing one millisecond.
//	second - A duration representing one second.
//	minute - A duration representing one minute.
//	hour - A duration representing one hour.
var Module = &starlarkstruct.Module{
	Name: "time",
	Members: starlark.StringDict{
		"from_timestamp":    starlark.NewBuiltin("from_timestamp", fromTimestamp),
		"is_valid_timezone": starlark.NewBuiltin("is_valid_timezone", isValidTimezone),
		"now":               starlark.NewBuiltin("now", now),
		"parse_duration":    starlark.NewBuiltin("parse_duration", parseDuration),
		"parse_time":        starlark.NewBuiltin("parse_time", parseTime),
		"time":              starlark.NewBuiltin("time", newTime),

		"nanosecond":  Duration(time.Nanosecond),
		"microsecond": Duration(time.Microsecond),
		"millisecond": Duration(time.Millisecond),
		"second":      Duration(time.Second),
		"minute":      Duration(time.Minute),
		"hour":        Duration(time.Hour),
	},
}
var safeties = map[string]starlark.SafetyFlags{
	"from_timestamp":    starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
	"is_valid_timezone": starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
	"now":               starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
	"parse_duration":    starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
	"parse_time":        starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
	"time":              starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
}

func init() {
	for name, safety := range safeties {
		if v, ok := Module.Members[name]; ok {
			if builtin, ok := v.(*starlark.Builtin); ok {
				builtin.DeclareSafety(safety)
			}
		}
	}
}

// NowFunc is a function that generates the current time. Intentionally exported
// so that it can be overridden, for example by applications that require their
// Starlark scripts to be fully deterministic.
var NowFunc = time.Now
var NowFuncSafety = starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe

func parseDuration(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	sdu := SafeDurationUnpacker{}
	sdu.BindThread(thread)
	if err := starlark.UnpackPositionalArgs("parse_duration", args, kwargs, 1, &sdu); err != nil {
		return nil, err
	}
	d := sdu.Duration()
	if err := thread.AddAllocs(starlark.EstimateSize(Duration(0))); err != nil {
		return nil, err
	}
	return d, nil
}

func isValidTimezone(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var s string
	if err := starlark.UnpackPositionalArgs("is_valid_timezone", args, kwargs, 1, &s); err != nil {
		return nil, err
	}
	_, err := time.LoadLocation(s)
	return starlark.Bool(err == nil), nil
}

func parseTime(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		x        string
		location = "UTC"
		format   = time.RFC3339
	)
	if err := starlark.UnpackArgs("parse_time", args, kwargs, "x", &x, "format?", &format, "location?", &location); err != nil {
		return nil, err
	}

	stepDelta := len(x)
	if formatLen := len(format); formatLen < stepDelta {
		stepDelta = formatLen
	}
	if err := thread.AddSteps(int64(stepDelta)); err != nil {
		return nil, err
	}

	if location == "UTC" {
		t, err := time.Parse(format, x)
		if err != nil {
			return nil, err
		}
		res := Time(t)
		if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
			return nil, err
		}
		return res, nil
	}

	loc, err := time.LoadLocation(location)
	if err != nil {
		return nil, err
	}
	t, err := time.ParseInLocation(format, x, loc)
	if err != nil {
		return nil, err
	}
	res := Time(t)
	if err := thread.AddAllocs(starlark.EstimateSize(res)); err != nil {
		return nil, err
	}
	return res, nil
}

func fromTimestamp(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		sec  int64
		nsec int64 = 0
	)
	if err := starlark.UnpackPositionalArgs("from_timestamp", args, kwargs, 1, &sec, &nsec); err != nil {
		return nil, err
	}
	if err := thread.AddAllocs(starlark.EstimateSize(Time{})); err != nil {
		return nil, err
	}
	return Time(time.Unix(sec, nsec)), nil
}

func now(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := thread.CheckPermits(NowFuncSafety); err != nil {
		return nil, err
	}
	if err := thread.AddAllocs(starlark.EstimateSize(Time{})); err != nil {
		return nil, err
	}
	return Time(NowFunc()), nil
}

// Duration is a Starlark representation of a duration.
type Duration time.Duration

// Assert at compile time that Duration implements Unpacker.
var _ starlark.Unpacker = (*Duration)(nil)

// Unpack is a custom argument unpacker
func (d *Duration) Unpack(v starlark.Value) error {
	switch x := v.(type) {
	case Duration:
		*d = x
		return nil
	case starlark.String:
		dur, err := time.ParseDuration(string(x))
		if err != nil {
			return err
		}

		*d = Duration(dur)
		return nil
	} // If more cases are added, be careful to update conversion cost computations.

	return fmt.Errorf("got %s, want a duration, string, or int", v.Type())
}

// SafeString implements the SafeStringer interface.
func (d Duration) SafeString(thread *starlark.Thread, sb starlark.StringBuilder) error {
	const safety = starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return err
	}
	// String conversion is cheap in both memory and time.
	// Hence, we can make this simple and accept a small spike.
	_, err := sb.WriteString(time.Duration(d).String())
	return err
}

// String implements the Stringer interface.
func (d Duration) String() string { return time.Duration(d).String() }

// Type returns a short string describing the value's type.
func (d Duration) Type() string { return "time.duration" }

// Freeze renders Duration immutable. required by starlark.Value interface
// because duration is already immutable this is a no-op.
func (d Duration) Freeze() {}

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y)
// required by starlark.Value interface.
func (d Duration) Hash() (uint32, error) {
	return uint32(d) ^ uint32(int64(d)>>32), nil
}

// Truth reports whether the duration is non-zero.
func (d Duration) Truth() starlark.Bool { return d != 0 }

func (d Duration) SafeAttr(thread *starlark.Thread, name string) (starlark.Value, error) {
	const safety = starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return nil, err
	}

	var result starlark.Value
	switch name {
	case "hours":
		result = starlark.Float(time.Duration(d).Hours())
	case "minutes":
		result = starlark.Float(time.Duration(d).Minutes())
	case "seconds":
		result = starlark.Float(time.Duration(d).Seconds())
	case "milliseconds":
		result = starlark.MakeInt64(time.Duration(d).Milliseconds())
	case "microseconds":
		result = starlark.MakeInt64(time.Duration(d).Microseconds())
	case "nanoseconds":
		result = starlark.MakeInt64(time.Duration(d).Nanoseconds())
	default:
		return nil, fmt.Errorf("unrecognized %s attribute %q", d.Type(), name)
	}
	if thread != nil {
		if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Attr gets a value for a string attribute, implementing dot expression support
// in starklark. required by starlark.HasAttrs interface.
func (d Duration) Attr(name string) (starlark.Value, error) {
	return d.SafeAttr(nil, name)
}

// AttrNames lists available dot expression strings. required by
// starlark.HasAttrs interface.
func (d Duration) AttrNames() []string {
	return []string{
		"hours",
		"minutes",
		"seconds",
		"milliseconds",
		"microseconds",
		"nanoseconds",
	}
}

// Cmp implements comparison of two Duration values. required by
// starlark.TotallyOrdered interface.
func (d Duration) Cmp(v starlark.Value, depth int) (int, error) {
	if x, y := d, v.(Duration); x < y {
		return -1, nil
	} else if x > y {
		return 1, nil
	}
	return 0, nil
}

// Binary implements binary operators, which satisfies the starlark.HasBinary
// interface. operators:
//
//	duration + duration = duration
//	duration + time = time
//	duration - duration = duration
//	duration / duration = float
//	duration / int = duration
//	duration / float = duration
//	duration // duration = int
//	duration * int = duration
func (d Duration) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	x := time.Duration(d)

	switch op {
	case syntax.PLUS:
		switch y := y.(type) {
		case Duration:
			return Duration(x + time.Duration(y)), nil
		case Time:
			return Time(time.Time(y).Add(x)), nil
		}

	case syntax.MINUS:
		switch y := y.(type) {
		case Duration:
			return Duration(x - time.Duration(y)), nil
		}

	case syntax.SLASH:
		switch y := y.(type) {
		case Duration:
			if y == 0 {
				return nil, fmt.Errorf("%s division by zero", d.Type())
			}
			return starlark.Float(x.Nanoseconds()) / starlark.Float(time.Duration(y).Nanoseconds()), nil
		case starlark.Int:
			if side == starlark.Right {
				return nil, fmt.Errorf("unsupported operation")
			}
			i, ok := y.Int64()
			if !ok {
				return nil, fmt.Errorf("int value out of range (want signed 64-bit value)")
			}
			if i == 0 {
				return nil, fmt.Errorf("%s division by zero", d.Type())
			}
			return d / Duration(i), nil
		case starlark.Float:
			f := float64(y)
			if f == 0 {
				return nil, fmt.Errorf("%s division by zero", d.Type())
			}
			return Duration(float64(x.Nanoseconds()) / f), nil
		}

	case syntax.SLASHSLASH:
		switch y := y.(type) {
		case Duration:
			if y == 0 {
				return nil, fmt.Errorf("%s division by zero", d.Type())
			}
			return starlark.MakeInt64(x.Nanoseconds() / time.Duration(y).Nanoseconds()), nil
		}

	case syntax.STAR:
		switch y := y.(type) {
		case starlark.Int:
			i, ok := y.Int64()
			if !ok {
				return nil, fmt.Errorf("int value out of range (want signed 64-bit value)")
			}
			return d * Duration(i), nil
		}
	}

	return nil, nil
}

type SafeDurationUnpacker struct {
	duration Duration
	thread   *starlark.Thread
}

func (sdu *SafeDurationUnpacker) Unpack(v starlark.Value) error {
	switch x := v.(type) {
	case Duration:
		sdu.duration = x
		return nil
	case starlark.String:
		if sdu.thread != nil {
			if err := sdu.thread.AddSteps(int64(len(string(x)))); err != nil {
				return err
			}
		}
		dur, err := time.ParseDuration(string(x))
		if err != nil {
			return err
		}
		sdu.duration = Duration(dur)
		return nil
	default:
		return fmt.Errorf("got %s, want a duration, string, or int", v.Type())
	}
}

// Duration returns the unpacked duration.
func (sdu *SafeDurationUnpacker) Duration() Duration {
	return sdu.duration
}

// BindThread causes this unpacker to report its resource
// usage to the given thread.
func (sdu *SafeDurationUnpacker) BindThread(thread *starlark.Thread) {
	sdu.thread = thread
}

// Time is a Starlark representation of a moment in time.
type Time time.Time

func newTime(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		year, month, day, hour, min, sec, nsec int
		loc                                    string
	)
	if err := starlark.UnpackArgs("time", args, kwargs,
		"year?", &year,
		"month?", &month,
		"day?", &day,
		"hour?", &hour,
		"minute?", &min,
		"second?", &sec,
		"nanosecond?", &nsec,
		"location?", &loc,
	); err != nil {
		return nil, err
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("time: unexpected positional arguments")
	}
	location, err := time.LoadLocation(loc)
	if err != nil {
		return nil, err
	}
	if location != time.UTC && location != time.Local {
		if err := thread.AddAllocs(starlark.EstimateSize(location)); err != nil {
			return nil, err
		}
	}
	if err := thread.AddAllocs(starlark.EstimateSize(time.Time{})); err != nil {
		return nil, err
	}
	res := starlark.Value(Time(time.Date(year, time.Month(month), day, hour, min, sec, nsec, location)))
	return res, nil
}

func (t Time) SafeString(thread *starlark.Thread, sb starlark.StringBuilder) error {
	const safety = starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return err
	}
	// String conversion is cheap in both memory and time.
	// Hence, we can make this simple and accept a small spike.
	_, err := sb.WriteString(t.String())
	return err
}

// String returns the time formatted using the format string
//
//	"2006-01-02 15:04:05.999999999 -0700 MST".
func (t Time) String() string { return time.Time(t).Format("2006-01-02 15:04:05.999999999 -0700 MST") }

// Type returns "time.time".
func (t Time) Type() string { return "time.time" }

// Freeze renders time immutable. required by starlark.Value interface
// because Time is already immutable this is a no-op.
func (t Time) Freeze() {}

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y)
// required by starlark.Value interface.
func (t Time) Hash() (uint32, error) {
	return uint32(time.Time(t).UnixNano()) ^ uint32(int64(time.Time(t).UnixNano())>>32), nil
}

// Truth returns the truth value of an object required by starlark.Value
// interface.
func (t Time) Truth() starlark.Bool { return !starlark.Bool(time.Time(t).IsZero()) }

func (t Time) SafeAttr(thread *starlark.Thread, name string) (starlark.Value, error) {
	const safety = starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe
	if err := starlark.CheckSafety(thread, safety); err != nil {
		return nil, err
	}

	var result starlark.Value
	switch name {
	case "year":
		result = starlark.MakeInt(time.Time(t).Year())
	case "month":
		result = starlark.MakeInt(int(time.Time(t).Month()))
	case "day":
		result = starlark.MakeInt(time.Time(t).Day())
	case "hour":
		result = starlark.MakeInt(time.Time(t).Hour())
	case "minute":
		result = starlark.MakeInt(time.Time(t).Minute())
	case "second":
		result = starlark.MakeInt(time.Time(t).Second())
	case "nanosecond":
		result = starlark.MakeInt(time.Time(t).Nanosecond())
	case "unix":
		result = starlark.MakeInt64(time.Time(t).Unix())
	case "unix_nano":
		result = starlark.MakeInt64(time.Time(t).UnixNano())
	default:
		if thread != nil {
			if err := thread.AddAllocs(starlark.EstimateSize(&time.Time{})); err != nil {
				return nil, err
			}
		}
		return safeBuiltinAttr(thread, t, name, timeMethods)
	}
	if thread != nil {
		if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Attr gets a value for a string attribute, implementing dot expression support
// in starklark. required by starlark.HasAttrs interface.
func (t Time) Attr(name string) (starlark.Value, error) {
	return t.SafeAttr(nil, name)
}

// AttrNames lists available dot expression strings for time. required by
// starlark.HasAttrs interface.
func (t Time) AttrNames() []string {
	return append(builtinAttrNames(timeMethods),
		"year",
		"month",
		"day",
		"hour",
		"minute",
		"second",
		"nanosecond",
		"unix",
		"unix_nano",
	)
}

// Cmp implements comparison of two Time values. Required by
// starlark.TotallyOrdered interface.
func (t Time) Cmp(yV starlark.Value, depth int) (int, error) {
	x := time.Time(t)
	y := time.Time(yV.(Time))
	if x.Before(y) {
		return -1, nil
	} else if x.After(y) {
		return 1, nil
	}
	return 0, nil
}

// Binary implements binary operators, which satisfies the starlark.HasBinary
// interface
//
//	time + duration = time
//	time - duration = time
//	time - time = duration
func (t Time) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	x := time.Time(t)

	switch op {
	case syntax.PLUS:
		switch y := y.(type) {
		case Duration:
			return Time(x.Add(time.Duration(y))), nil
		}
	case syntax.MINUS:
		switch y := y.(type) {
		case Duration:
			return Time(x.Add(time.Duration(-y))), nil
		case Time:
			// time - time = duration
			return Duration(x.Sub(time.Time(y))), nil
		}
	}

	return nil, nil
}

var timeMethods = map[string]builtinMethod{
	"in_location": timeIn,
	"format":      timeFormat,
}

var timeMethodSafeties = map[string]starlark.SafetyFlags{
	"in_location": starlark.MemSafe | starlark.CPUSafe,
	"format":      starlark.MemSafe | starlark.IOSafe | starlark.CPUSafe,
}

func timeFormat(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var x string
	if err := starlark.UnpackPositionalArgs("format", args, kwargs, 1, &x); err != nil {
		return nil, err
	}

	if err := thread.AddSteps(int64(len(x))); err != nil {
		return nil, err
	}
	if err := thread.CheckAllocs(int64(len(x))); err != nil {
		return nil, err
	}
	recv := time.Time(b.Receiver().(Time))
	result := starlark.Value(starlark.String(recv.Format(x)))
	if err := thread.AddAllocs(starlark.EstimateSize(result)); err != nil {
		return nil, err
	}
	return result, nil
}

func timeIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var x string
	if err := starlark.UnpackPositionalArgs("in_location", args, kwargs, 1, &x); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(x)
	if err != nil {
		return nil, err
	}
	if loc != time.UTC && loc != time.Local {
		if err := thread.AddAllocs(starlark.EstimateSize(loc)); err != nil {
			return nil, err
		}
	}
	if err := thread.AddAllocs(starlark.EstimateSize(time.Time{})); err != nil {
		return nil, err
	}
	recv := time.Time(b.Receiver().(Time))
	return Time(recv.In(loc)), nil
}

type builtinMethod func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

func safeBuiltinAttr(thread *starlark.Thread, recv starlark.Value, name string, methods map[string]builtinMethod) (starlark.Value, error) {
	method := methods[name]
	if method == nil {
		return nil, starlark.ErrNoSuchAttr
	}
	if thread != nil {
		if err := thread.AddAllocs(starlark.EstimateSize(&starlark.Builtin{})); err != nil {
			return nil, err
		}
	}
	b := starlark.NewBuiltin(name, method).BindReceiver(recv)
	b.DeclareSafety(timeMethodSafeties[name])
	return b, nil
}

func builtinAttrNames(methods map[string]builtinMethod) []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
