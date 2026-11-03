package appointments

import "testing"

func TestNextStatus(t *testing.T) {
	cases := []struct {
		cur  Status
		ev   Event
		want Status
	}{
		{StatusIdle, EventProvideService, StatusPendingAvailability},
		{StatusPendingService, EventProvideService, StatusPendingAvailability},
		{StatusPendingService, EventProvideAvailability, StatusReady},
		{StatusPendingAvailability, EventProvideAvailability, StatusReady},
		{StatusReady, EventConfirm, StatusFinish},
		{StatusReady, EventFinish, StatusFinish},
		{StatusReady, EventCancel, StatusCancel},
		{StatusIdle, EventConfirm, StatusIdle},
	}
	for _, c := range cases {
		if got := NextStatus(c.cur, c.ev); got != c.want {
			t.Errorf("NextStatus(%q, %q) = %q, quiero %q", c.cur, c.ev, got, c.want)
		}
	}
}
