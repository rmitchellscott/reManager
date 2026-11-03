package main

import (
	"math"
	"testing"
	"time"
)

func TestEstimatedFractionMatchesTheFrontendCurve(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    float64
	}{
		{0, 0},
		{30 * time.Second, 0.40},
		{60 * time.Second, 0.80},
		{120 * time.Second, 0.875},
		{time.Hour, 0.95 - 0.15/60},
	}
	for _, c := range cases {
		if got := estimatedFraction(c.elapsed, 60); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("estimatedFraction(%v, 60) = %v, want %v", c.elapsed, got, c.want)
		}
	}
}
