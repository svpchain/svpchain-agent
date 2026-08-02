package update

import (
	"testing"
	"time"
)

func TestThrottleProgress(t *testing.T) {
	var calls int
	var lastDone int64
	fn := throttleProgress(func(done, total int64) {
		calls++
		lastDone = done
	}, 100*time.Millisecond)

	for i := int64(1); i < 100; i++ {
		fn(i, 100)
	}
	if calls == 0 {
		t.Fatal("expected at least one progress call")
	}
	if lastDone >= 100 {
		t.Fatalf("lastDone = %d, want < 100 before final call", lastDone)
	}

	fn(100, 100)
	if lastDone != 100 {
		t.Fatalf("lastDone = %d, want 100", lastDone)
	}
}

func TestScaleProgress(t *testing.T) {
	var gotDone, gotTotal int64
	base := scaleProgress(func(done, total int64) {
		gotDone = done
		gotTotal = total
	}, 100, 200, 1000)

	base(50, 100)
	if gotDone != 200 || gotTotal != 1000 {
		t.Fatalf("scaled progress = (%d, %d), want (200, 1000)", gotDone, gotTotal)
	}
}
