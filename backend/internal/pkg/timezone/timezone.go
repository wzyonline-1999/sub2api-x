// Package timezone provides global timezone management for the application.
// Similar to PHP's date_default_timezone_set, this package allows setting
// a global timezone that affects all time.Now() calls.
package timezone

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// ResolvedUserLocation is a validated request timezone. Its location is kept
// private so callers cannot place an unchecked timezone name in a context that
// is later consumed by SQL grouping code.
type ResolvedUserLocation struct {
	location *time.Location
}

type resolvedUserLocationContextKey struct{}

// ResolveUserLocation resolves a caller-provided IANA timezone and safely
// falls back to the configured server timezone when it is empty or invalid.
func ResolveUserLocation(userTZ string) ResolvedUserLocation {
	loc := fallbackUserLocation()
	if name := strings.TrimSpace(userTZ); name != "" && name != "Local" {
		if userLoc, err := time.LoadLocation(name); err == nil {
			loc = userLoc
		}
	}
	return ResolvedUserLocation{location: loc}
}

func fallbackUserLocation() *time.Location {
	if loc := Location(); loc != nil && loc.String() != "Local" {
		return loc
	}
	if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" && envTZ != "Local" {
		if loc, err := time.LoadLocation(envTZ); err == nil {
			return loc
		}
	}
	return time.UTC
}

// Location returns the validated location, or the configured server location
// for a zero-value ResolvedUserLocation.
func (r ResolvedUserLocation) Location() *time.Location {
	if r.location == nil {
		return fallbackUserLocation()
	}
	return r.location
}

// Name returns the canonical name of the validated location.
func (r ResolvedUserLocation) Name() string {
	return r.Location().String()
}

// WithResolvedUserLocation carries a validated request timezone through
// handler, service, and repository layers without changing repository APIs.
func WithResolvedUserLocation(ctx context.Context, resolved ResolvedUserLocation) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	resolved.location = resolved.Location()
	return context.WithValue(ctx, resolvedUserLocationContextKey{}, resolved)
}

// ResolvedUserLocationFromContext returns a previously validated request
// timezone. The boolean is false when no request timezone was attached.
func ResolvedUserLocationFromContext(ctx context.Context) (ResolvedUserLocation, bool) {
	if ctx == nil {
		return ResolvedUserLocation{}, false
	}
	resolved, ok := ctx.Value(resolvedUserLocationContextKey{}).(ResolvedUserLocation)
	if !ok || resolved.location == nil {
		return ResolvedUserLocation{}, false
	}
	return resolved, true
}

var (
	// location is the global timezone location
	location *time.Location
	// tzName stores the timezone name for logging/debugging
	tzName string
)

// Init initializes the global timezone setting.
// This should be called once at application startup.
// Example timezone values: "Asia/Shanghai", "America/New_York", "UTC"
func Init(tz string) error {
	if tz == "" {
		tz = "Asia/Shanghai" // Default timezone
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", tz, err)
	}

	// Set the global Go time.Local to our timezone
	// This affects time.Now() throughout the application
	time.Local = loc
	location = loc
	tzName = tz

	log.Printf("Timezone initialized: %s (UTC offset: %s)", tz, getUTCOffset(loc))
	return nil
}

// getUTCOffset returns the current UTC offset for a location
func getUTCOffset(loc *time.Location) string {
	_, offset := time.Now().In(loc).Zone()
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	if minutes < 0 {
		minutes = -minutes
	}
	sign := "+"
	if hours < 0 {
		sign = "-"
		hours = -hours
	}
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

// Now returns the current time in the configured timezone.
// This is equivalent to time.Now() after Init() is called,
// but provided for explicit timezone-aware code.
func Now() time.Time {
	if location == nil {
		return time.Now()
	}
	return time.Now().In(location)
}

// Location returns the configured timezone location.
func Location() *time.Location {
	if location == nil {
		return time.Local
	}
	return location
}

// Name returns the configured timezone name.
func Name() string {
	if tzName == "" {
		return "Local"
	}
	return tzName
}

// UTCOffset returns the current UTC offset of the configured timezone, e.g. "+08:00".
func UTCOffset() string {
	return getUTCOffset(Location())
}

// StartOfDay returns the start of the given day (00:00:00) in the configured timezone.
func StartOfDay(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// Today returns the start of today (00:00:00) in the configured timezone.
func Today() time.Time {
	return StartOfDay(Now())
}

// EndOfDay returns the end of the given day (23:59:59.999999999) in the configured timezone.
func EndOfDay(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc)
}

// StartOfWeek returns the start of the week (Monday 00:00:00) for the given time.
func StartOfWeek(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday is day 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-weekday+1, 0, 0, 0, 0, loc)
}

// StartOfMonth returns the start of the month (1st day 00:00:00) for the given time.
func StartOfMonth(t time.Time) time.Time {
	loc := Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
}

// ParseInLocation parses a time string in the configured timezone.
func ParseInLocation(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, Location())
}

// ParseInUserLocation parses a time string in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func ParseInUserLocation(layout, value, userTZ string) (time.Time, error) {
	return time.ParseInLocation(layout, value, ResolveUserLocation(userTZ).Location())
}

// NowInUserLocation returns the current time in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func NowInUserLocation(userTZ string) time.Time {
	return time.Now().In(ResolveUserLocation(userTZ).Location())
}

// StartOfDayInUserLocation returns the start of the given day in the user's timezone.
// If userTZ is empty or invalid, falls back to the configured server timezone.
func StartOfDayInUserLocation(t time.Time, userTZ string) time.Time {
	loc := ResolveUserLocation(userTZ).Location()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
