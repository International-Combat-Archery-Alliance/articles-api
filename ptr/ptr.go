package ptr

import "time"

func Int(i int) *int {
	return &i
}

func String(s string) *string {
	return &s
}

func Time(t time.Time) *time.Time {
	return &t
}
