package pgdb

import (
	"testing"
	"time"
)

func TestTimeHelpers(t *testing.T) {
	early := time.Date(2026, 10, 18, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	late := early.Add(time.Hour)
	for _, tc := range []struct {
		name   string
		times  []*time.Time
		latest time.Time
	}{
		{"none", nil, time.Time{}},
		{"nil and zero are unset", []*time.Time{nil, {}}, time.Time{}},
		{"latest wins", []*time.Time{&late, nil, &early}, late},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Latest(tc.times...); !got.Equal(tc.latest) {
				t.Fatalf("Latest = %v, want %v", got, tc.latest)
			}
		})
	}
	if NullTime(time.Time{}) != nil {
		t.Fatal("NullTime(zero) is not nil")
	}
	if got := NullTime(early); got == nil || !got.Equal(early) {
		t.Fatalf("NullTime(%v) = %v", early, got)
	}
	if UTC(nil) != nil {
		t.Fatal("UTC(nil) is not nil")
	}
	if got := UTC(&early); got.Location() != time.UTC || !got.Equal(early) {
		t.Fatalf("UTC(%v) = %v", early, got)
	}
}
