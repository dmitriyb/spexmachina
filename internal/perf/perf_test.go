package perf

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWithinPassesOnFirstTryWithoutRetry(t *testing.T) {
	if RaceEnabled {
		t.Skip("budget assertions are not exercised under the race detector")
	}
	calls := 0
	Within(t, 200*time.Millisecond, func() {
		calls++
	})
	if calls != 1 {
		t.Fatalf("want fn called once when the first try is within budget, got %d calls", calls)
	}
}

func TestWithinRetriesOnceOnOvershootThenPasses(t *testing.T) {
	if RaceEnabled {
		t.Skip("budget assertions are not exercised under the race detector")
	}
	calls := 0
	Within(t, 150*time.Millisecond, func() {
		calls++
		if calls == 1 {
			time.Sleep(400 * time.Millisecond)
		}
	})
	if calls != 2 {
		t.Fatalf("want fn called twice (initial overshoot + retry), got %d calls", calls)
	}
}

// TestWithinFailsWhenRetryAlsoOvershoots checks that the retry only forgives
// a lone outlier, not genuine slowness. It re-execs this test binary because
// Within calls t.Fatalf, which would otherwise fail this process too.
func TestWithinFailsWhenRetryAlsoOvershoots(t *testing.T) {
	if os.Getenv("PERF_TEST_WANT_OVERSHOOT") == "1" {
		Within(t, 150*time.Millisecond, func() {
			time.Sleep(400 * time.Millisecond)
		})
		return
	}
	if RaceEnabled {
		t.Skip("budget assertions are not exercised under the race detector")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWithinFailsWhenRetryAlsoOvershoots$")
	cmd.Env = append(os.Environ(), "PERF_TEST_WANT_OVERSHOOT=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want Within to fail when both the initial try and the retry overshoot the budget, got success:\n%s", out)
	}
}
