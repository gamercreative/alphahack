package services

import "testing"

// govListed returns a lookup that finds only the listed (ingredient, strength, form) keys.
func govListed(listed ...GovProduct) func(string, string, string) (GovProduct, bool) {
	return func(ingredient, strength, form string) (GovProduct, bool) {
		for _, p := range listed {
			if normDoseIngredient(p.Ingredient) == normDoseIngredient(ingredient) &&
				sameDoseStrength(p.Strength, strength) && normDoseForm(p.DosageForm) == normDoseForm(form) {
				return p, true
			}
		}
		return GovProduct{}, false
	}
}

// sameDoseStrength compares two strength strings after parsing (so "1g" == "1000mg").
func sameDoseStrength(a, b string) bool {
	av, au, okA := doseStrength(a)
	bv, bu, okB := doseStrength(b)
	return okA && okB && au == bu && av == bv
}

func TestDoseStrength(t *testing.T) {
	tests := []struct {
		in    string
		value float64
		unit  string
		ok    bool
	}{
		{"1000mg", 1000, "mg", true},
		{"2.5 mg", 2.5, "mg", true},
		{"100mcg", 100, "mcg", true},
		{"1g", 1000, "mg", true},
		{"  1000 MG ", 1000, "mg", true},
		{"0.5G", 500, "mg", true},
		{"", 0, "", false},
		{"mg", 0, "", false},
		{"500", 0, "", false},
		{"0mg", 0, "", false},
		{"-5mg", 0, "", false},
		{"500ml", 0, "", false},
		{"1.2.3mg", 0, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			v, u, ok := doseStrength(tc.in)
			if ok != tc.ok || u != tc.unit || v != tc.value {
				t.Fatalf("doseStrength(%q) = (%v, %q, %v), want (%v, %q, %v)",
					tc.in, v, u, ok, tc.value, tc.unit, tc.ok)
			}
		})
	}
}

func TestDoseStrengthFormat(t *testing.T) {
	tests := []struct {
		value float64
		unit  string
		want  string
	}{
		{500, "mg", "500mg"},
		{2.5, "mg", "2.5mg"},
		{1000, "mg", "1000mg"},
		{100, "mcg", "100mcg"},
		{0.25, "mg", "0.25mg"},
		{2.50, "mg", "2.5mg"},
		{1.005, "mg", "1mg"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := formatDoseStrength(tc.value, tc.unit); got != tc.want {
				t.Fatalf("formatDoseStrength(%v, %q) = %q, want %q", tc.value, tc.unit, got, tc.want)
			}
		})
	}
}

func TestEvaluateEquivalence(t *testing.T) {
	paracetamol1000 := GovProduct{Code: "PCM-500", Ingredient: "Paracetamol", Strength: "500mg", DosageForm: "tablet"}
	paracetamol1000g := GovProduct{Code: "PCM-1G", Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet"}
	amoxClav := GovProduct{Code: "AMC-875", Ingredient: "Amoxicillin/Clavulanate", Strength: "875mg", DosageForm: "tablet"}
	diazepam5 := GovProduct{Code: "DZ-5", Ingredient: "Diazepam", Strength: "5mg", DosageForm: "tablet"}
	diazepam2_5 := GovProduct{Code: "DZ-2.5", Ingredient: "Diazepam", Strength: "2.5mg", DosageForm: "tablet"}
	all := govListed(paracetamol1000, paracetamol1000g, amoxClav, diazepam5, diazepam2_5)

	tests := []struct {
		name     string
		req      DoseRequest
		off      DoseOffer
		lookup   func(string, string, string) (GovProduct, bool)
		accepted bool
		units    int
		govCode  string
		failing  string // label expected to fail, "" when all pass
	}{
		{
			name:     "2x500mg for 1000mg x2 with qty 4 accepted",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 2},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "500mg", DosageForm: "Tab", Quantity: 4},
			lookup:   all,
			accepted: true, units: 2, govCode: "PCM-500",
		},
		{
			name:     "625mg for 1000mg rejected (Total false)",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "625mg", DosageForm: "tablet", Quantity: 4},
			lookup:   govListed(GovProduct{Code: "PCM-625", Ingredient: "Paracetamol", Strength: "625mg", DosageForm: "tablet"}),
			accepted: false, units: 0, govCode: "PCM-625", failing: DoseLabelTotal,
		},
		{
			name:     "1g vs 1000mg same strength accepted",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "1g", DosageForm: "tablet", Quantity: 1},
			lookup:   govListed(GovProduct{Code: "PCM-1G", Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet"}),
			accepted: true, units: 1, govCode: "PCM-1G",
		},
		{
			name:     "2.5mg x2 for 5mg accepted when listed",
			req:      DoseRequest{Ingredient: "Diazepam", Strength: "5mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Diazepam", UnitStrength: "2.5mg", DosageForm: "tablet", Quantity: 2},
			lookup:   all,
			accepted: true, units: 2, govCode: "DZ-2.5",
		},
		{
			name:     "unlisted half strength rejected (Gov false)",
			req:      DoseRequest{Ingredient: "Diazepam", Strength: "5mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Diazepam", UnitStrength: "2.5mg", DosageForm: "tablet", Quantity: 2},
			lookup:   govListed(diazepam5),
			accepted: false, units: 2, govCode: "", failing: DoseLabelGov,
		},
		{
			name:     "different ingredient rejected",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Ibuprofen", UnitStrength: "500mg", DosageForm: "tablet", Quantity: 2},
			lookup:   govListed(GovProduct{Code: "IBU-500", Ingredient: "Ibuprofen", Strength: "500mg", DosageForm: "tablet"}),
			accepted: false, units: 2, govCode: "IBU-500", failing: DoseLabelIngredient,
		},
		{
			name:     "capsule vs tablet rejected",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "capsule", Quantity: 1},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "500mg", DosageForm: "tablet", Quantity: 2},
			lookup:   all,
			accepted: false, units: 2, govCode: "PCM-500", failing: DoseLabelForm,
		},
		{
			name:     "qty too small rejected",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 2},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "500mg", DosageForm: "tablet", Quantity: 3},
			lookup:   all,
			accepted: false, units: 0, govCode: "PCM-500", failing: DoseLabelTotal,
		},
		{
			name:     "5 units per dose rejected (n>4)",
			req:      DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Paracetamol", UnitStrength: "200mg", DosageForm: "tablet", Quantity: 10},
			lookup:   govListed(GovProduct{Code: "PCM-200", Ingredient: "Paracetamol", Strength: "200mg", DosageForm: "tablet"}),
			accepted: false, units: 0, govCode: "PCM-200", failing: DoseLabelTotal,
		},
		{
			name:     "ingredient order and separator variants accepted",
			req:      DoseRequest{Ingredient: "Amoxicillin/Clavulanate", Strength: "875mg", DosageForm: "film-coated tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "clavulanate + amoxicillin", UnitStrength: "875mg", DosageForm: "tablets", Quantity: 1},
			lookup:   all,
			accepted: true, units: 1, govCode: "AMC-875",
		},
		{
			name:     "ingredient written with 'and' accepted",
			req:      DoseRequest{Ingredient: "Amoxicillin and Clavulanate", Strength: "875mg", DosageForm: "tablet", Quantity: 1},
			off:      DoseOffer{Ingredient: "Clavulanate/Amoxicillin", UnitStrength: "875mg", DosageForm: "tablet", Quantity: 1},
			lookup:   all,
			accepted: true, units: 1, govCode: "AMC-875",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dc := EvaluateEquivalence(tc.req, tc.off, tc.lookup)
			if dc.Accepted != tc.accepted {
				t.Fatalf("Accepted = %v, want %v (checks %+v)", dc.Accepted, tc.accepted, dc.Checks)
			}
			if dc.UnitsPerDose != tc.units {
				t.Fatalf("UnitsPerDose = %d, want %d", dc.UnitsPerDose, tc.units)
			}
			if dc.GovCode != tc.govCode {
				t.Fatalf("GovCode = %q, want %q", dc.GovCode, tc.govCode)
			}
			if len(dc.Checks) != 4 {
				t.Fatalf("got %d checks, want 4", len(dc.Checks))
			}
			wantLabels := []string{DoseLabelIngredient, DoseLabelForm, DoseLabelTotal, DoseLabelGov}
			for i, c := range dc.Checks {
				if c.Label != wantLabels[i] {
					t.Fatalf("check %d label = %q, want %q", i, c.Label, wantLabels[i])
				}
				wantOK := c.Label != tc.failing
				if c.OK != wantOK {
					t.Fatalf("check %q OK = %v, want %v", c.Label, c.OK, wantOK)
				}
			}
		})
	}
}

func TestEvaluateEquivalenceDisplayStrings(t *testing.T) {
	dc := EvaluateEquivalence(
		DoseRequest{Ingredient: "Paracetamol", Strength: "1000mg", DosageForm: "tablet", Quantity: 2},
		DoseOffer{Ingredient: "Paracetamol", UnitStrength: "500mg", DosageForm: "tablet", Quantity: 4},
		govListed(GovProduct{Code: "PCM-500", Ingredient: "Paracetamol", Strength: "500mg", DosageForm: "tablet"}),
	)
	if dc.Requested != "1000mg × 2" {
		t.Fatalf("Requested = %q", dc.Requested)
	}
	if dc.Offered != "500mg × 4" {
		t.Fatalf("Offered = %q", dc.Offered)
	}
}
