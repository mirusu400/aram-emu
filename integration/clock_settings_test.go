package integration

import (
	"testing"
	"time"

	"github.com/mirusu400/aram-core/application"
	"github.com/mirusu400/aram-frontend/frontend"
)

func factoryWallEpoch(t *testing.T, backend *Backend) int64 {
	t.Helper()
	factory, ok := backend.factoryForCreate().(application.Factory)
	if !ok {
		t.Fatal("default factory is not application.Factory")
	}
	return factory.WallEpochMillis
}

// A backend no frontend configured keeps the core's fixed start date, which is
// what keeps headless probes reproducible. Once a frontend selects the handset
// clock, each machine starts at the device's time moved by the chosen offset,
// sampled when the machine is created.
func TestHandsetClockReachesApplicationFactory(t *testing.T) {
	seoul := time.FixedZone("KST", 9*60*60)
	now := time.Date(2026, time.November, 26, 14, 3, 20, 0, seoul)
	previous := hostNow
	hostNow = func() time.Time { return now }
	t.Cleanup(func() { hostNow = previous })

	backend := NewBackend(nil)
	if got := factoryWallEpoch(t, backend); got != 0 {
		t.Fatalf("unconfigured backend wall epoch = %d, want the core default", got)
	}

	if err := backend.ConfigureClock(frontend.ClockSettings{}); err != nil {
		t.Fatal(err)
	}
	if got, want := factoryWallEpoch(t, backend), now.UnixMilli(); got != want {
		t.Fatalf("device-time wall epoch = %d, want %d", got, want)
	}

	// The time is sampled at creation, so a title opened later starts later.
	now = now.Add(time.Hour)
	if got, want := factoryWallEpoch(t, backend), now.UnixMilli(); got != want {
		t.Fatalf("wall epoch an hour later = %d, want %d", got, want)
	}

	offset := -24 * time.Hour
	if err := backend.ConfigureClock(frontend.ClockSettings{Offset: offset}); err != nil {
		t.Fatal(err)
	}
	if got, want := factoryWallEpoch(t, backend), now.Add(offset).UnixMilli(); got != want {
		t.Fatalf("offset wall epoch = %d, want %d", got, want)
	}

	if err := backend.ConfigureClock(frontend.ClockSettings{Offset: -100 * 365 * 24 * time.Hour}); err == nil {
		t.Fatal("an offset reaching before 1970 was accepted")
	}
}

// The guest handset keeps Korean time. A device in another zone must still see
// its own local date and time in a title, so its zone is folded into the epoch.
func TestHandsetClockShowsTheDeviceLocalTime(t *testing.T) {
	newYork := time.FixedZone("EST", -5*60*60)
	now := time.Date(2026, time.November, 26, 14, 3, 20, 0, newYork)
	epoch := guestWallEpochMillis(now, 0)

	guestLocal := time.UnixMilli(epoch).In(time.FixedZone("KST", 9*60*60))
	if guestLocal.Hour() != 14 || guestLocal.Minute() != 3 || guestLocal.Day() != 26 {
		t.Fatalf("guest calendar shows %s, want the device's 26th 14:03", guestLocal)
	}
}
