package apihttp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// minBackupCronInterval is the shortest allowed gap, in minutes, between two
// runs of a scheduled backup. Anything more frequent (e.g. "* * * * *") just
// piles up jobs and fills storage.
const minBackupCronInterval = 15

// validateBackupCron checks that expr is a well-formed 5-field cron
// expression (minute hour day-of-month month day-of-week; numbers, "*",
// ranges "a-b", steps "*/n" or "a-b/n", comma lists) and that it never fires
// more often than every minBackupCronInterval minutes.
func validateBackupCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron_expr must have 5 fields (minute hour day month weekday)")
	}
	bounds := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	names := [5]string{"minute", "hour", "day-of-month", "month", "day-of-week"}
	var minutes []int
	for i, f := range fields {
		set, err := parseCronFieldStrict(f, bounds[i][0], bounds[i][1])
		if err != nil {
			return fmt.Errorf("invalid cron %s field %q: %v", names[i], f, err)
		}
		if i == 0 {
			minutes = set
		}
	}
	// Smallest circular gap between the minutes the schedule fires at. With a
	// single minute the schedule fires at most hourly.
	gap := 60
	for i := range minutes {
		next := minutes[(i+1)%len(minutes)]
		d := next - minutes[i]
		if d <= 0 {
			d += 60
		}
		if len(minutes) > 1 && d < gap {
			gap = d
		}
	}
	if gap < minBackupCronInterval {
		return fmt.Errorf("backup schedule may not run more often than every %d minutes", minBackupCronInterval)
	}
	return nil
}

// parseCronFieldStrict expands one cron field into its sorted set of values,
// rejecting anything malformed or out of range.
func parseCronFieldStrict(field string, lo, hi int) ([]int, error) {
	set := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return nil, fmt.Errorf("empty list element")
		}
		rangePart, step := part, 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid step")
			}
			rangePart, step = part[:i], n
		}
		start, end := lo, hi
		switch {
		case rangePart == "*":
		case strings.Contains(rangePart, "-"):
			ab := strings.SplitN(rangePart, "-", 2)
			a, errA := strconv.Atoi(ab[0])
			b, errB := strconv.Atoi(ab[1])
			if errA != nil || errB != nil || a > b {
				return nil, fmt.Errorf("invalid range")
			}
			start, end = a, b
		default:
			v, err := strconv.Atoi(rangePart)
			if err != nil {
				return nil, fmt.Errorf("invalid value")
			}
			start = v
			if step == 1 {
				end = v
			}
		}
		if start < lo || end > hi {
			return nil, fmt.Errorf("value out of range %d-%d", lo, hi)
		}
		for v := start; v <= end; v += step {
			set[v] = true
		}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out, nil
}
