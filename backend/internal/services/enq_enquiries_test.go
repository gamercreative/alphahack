package services

import (
	"math"
	"testing"
)

func TestHaversineKm(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lng1, lat2, lng2 float64
		wantKm, tolKm          float64
	}{
		{"same point is zero", 33.8959, 35.4787, 33.8959, 35.4787, 0, 1e-9},
		// Hamra to Baabda, Beirut area: about 9.2 km.
		{"hamra to baabda", 33.8959, 35.4787, 33.8339, 35.5442, 9.2, 0.5},
		{"hamra to baabda reversed", 33.8339, 35.5442, 33.8959, 35.4787, 9.2, 0.5},
		{"one degree of latitude", 0, 0, 1, 0, 111.19, 0.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := haversineKm(tt.lat1, tt.lng1, tt.lat2, tt.lng2)
			if math.IsNaN(got) || math.Abs(got-tt.wantKm) > tt.tolKm {
				t.Fatalf("haversineKm = %.4f km, want %.2f +- %.2f", got, tt.wantKm, tt.tolKm)
			}
		})
	}
}
