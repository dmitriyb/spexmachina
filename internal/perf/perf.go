// Package perf holds the one wall-clock budget assertion tests are allowed to
// make. The spec's performance scenarios pin budgets of the uninstrumented
// binary; under a race-instrumented build the detector's slowdown makes
// wall-clock a measure of instrumentation, not of the code, so Within stands
// down there and the plain test job remains the place every budget is
// asserted. Only imported from _test.go files.
package perf

import (
	"testing"
	"time"
)

// Within runs fn and asserts it finished inside budget. In a race-instrumented
// build the elapsed time is logged and the assertion is skipped.
func Within(t *testing.T, budget time.Duration, fn func()) {
	t.Helper()
	start := time.Now()
	fn()
	elapsed := time.Since(start)
	if RaceEnabled {
		t.Logf("perf: %s elapsed (budget %s) — not asserted under the race detector", elapsed, budget)
		return
	}
	if elapsed > budget {
		t.Fatalf("perf: want completion under %s, took %s", budget, elapsed)
	}
}
