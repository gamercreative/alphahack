package services

import "testing"

func TestCostMicros(t *testing.T) {
	cases := []struct {
		name              string
		in, out           int
		priceIn, priceOut float64
		want              int64
	}{
		{"one million input tokens", 1_000_000, 0, 0.2, 0.8, 200000},
		{"small mixed run", 1000, 500, 0.2, 0.8, 600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := costMicros(c.in, c.out, c.priceIn, c.priceOut); got != c.want {
				t.Fatalf("costMicros(%d, %d, %v, %v) = %d, want %d",
					c.in, c.out, c.priceIn, c.priceOut, got, c.want)
			}
		})
	}
}
