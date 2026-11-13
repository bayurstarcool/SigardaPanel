package store

import (
	"testing"
	"time"
)

func TestNextCronRunSundaySevenAndRangeStep(t *testing.T) {
	loc := time.UTC
	friday := time.Date(2026, 11, 13, 10, 0, 0, 0, loc) // a Friday
	if got := NextCronRun("0 3 * * 7", friday); got.Weekday() != time.Sunday || got.Hour() != 3 {
		t.Fatalf("0 3 * * 7 → %v, want Sunday 03:00", got)
	}
	if got := NextCronRun("0 3 * * 0", friday); got.Weekday() != time.Sunday {
		t.Fatalf("0 3 * * 0 → %v", got)
	}
	early := time.Date(2026, 11, 13, 1, 0, 0, 0, loc)
	if got := NextCronRun("0 9-17/4 * * *", early); got.Hour() != 9 {
		t.Fatalf("0 9-17/4 → %v, want 09:00", got)
	}
	afterNine := time.Date(2026, 11, 13, 9, 30, 0, 0, loc)
	if got := NextCronRun("0 9-17/4 * * *", afterNine); got.Hour() != 13 {
		t.Fatalf("0 9-17/4 after 09:30 → %v, want 13:00", got)
	}
}
