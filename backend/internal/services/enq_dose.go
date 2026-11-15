package services

// Dose equivalence rules (CONTRACT rule 1). A pharmacy's offer of a different
// strength counts as a match only when every check below passes. Pure functions:
// no database, no HTTP. The model never decides equivalence.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// GovProduct is one line of the government reference list.
type GovProduct struct {
	Code       string
	BrandName  string
	Ingredient string
	Strength   string
	DosageForm string
}

// DoseRequest is the requested medicine. Quantity = boxes/units requested.
type DoseRequest struct {
	Ingredient string
	Strength   string
	DosageForm string
	Quantity   int
}

// DoseOffer is what the pharmacy offered. Quantity = units offered.
type DoseOffer struct {
	Ingredient   string
	UnitStrength string
	DosageForm   string
	Quantity     int
}

// DoseCheckItem is one labelled check result.
type DoseCheckItem struct {
	Label string `json:"label"`
	OK    bool   `json:"ok"`
}

// DoseCheck is the full equivalence verdict.
type DoseCheck struct {
	Accepted     bool            `json:"accepted"`
	Requested    string          `json:"requested"`
	Offered      string          `json:"offered"`
	UnitsPerDose int             `json:"units_per_dose"`
	Checks       []DoseCheckItem `json:"checks"`
	GovCode      string          `json:"gov_code"`
}

// Labels of the four checks, always returned in this order.
const (
	DoseLabelIngredient = "Same active ingredient"
	DoseLabelForm       = "Same form"
	DoseLabelTotal      = "Total strength matches"
	DoseLabelGov        = "Listed in government reference"
)

// maxDoseUnitsPerDose is the largest whole number of offered units per requested dose.
const maxDoseUnitsPerDose = 4

// doseStrength parses "1000mg", "2.5 mg", "100mcg" or "1g" (converted to 1000 mg).
// Case-insensitive, spaces allowed. Only positive values are accepted.
func doseStrength(s string) (value float64, unit string, ok bool) {
	compact := strings.ToLower(strings.Join(strings.Fields(s), ""))
	i := 0
	for i < len(compact) && ((compact[i] >= '0' && compact[i] <= '9') || compact[i] == '.') {
		i++
	}
	if i == 0 || i == len(compact) {
		return 0, "", false
	}
	num, err := strconv.ParseFloat(compact[:i], 64)
	if err != nil || math.IsNaN(num) || math.IsInf(num, 0) || num <= 0 {
		return 0, "", false
	}
	switch compact[i:] {
	case "mg":
		return num, "mg", true
	case "mcg":
		return num, "mcg", true
	case "g":
		return num * 1000, "mg", true
	}
	return 0, "", false
}

// formatDoseStrength renders 500,"mg" as "500mg" and 2.5,"mg" as "2.5mg"
// (at most 2 decimals, trailing zeros stripped).
func formatDoseStrength(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return s + unit
}

// normDoseIngredient makes "Amoxicillin/Clavulanate" and "clavulanate + amoxicillin"
// equal: lowercase, "/" and " and " are the same separator as "+", parts sorted.
func normDoseIngredient(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " and ", "+")
	s = strings.ReplaceAll(s, "/", "+")
	var parts []string
	for _, p := range strings.Split(s, "+") {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			parts = append(parts, p)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " + ")
}

// normDoseForm maps the dosage form words that mean the same thing onto one name.
func normDoseForm(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	switch s {
	case "tab", "tabs", "tablet", "tablets", "film-coated tablet":
		return "tablet"
	case "caps", "capsule", "capsules":
		return "capsule"
	}
	return s
}

// doseTotal checks that the offered units add up exactly to the requested strength.
// It returns the units per dose n (req strength / unit strength) when accepted.
func doseTotal(req DoseRequest, off DoseOffer) (bool, int) {
	if req.Quantity <= 0 {
		return false, 0
	}
	rv, ru, okReq := doseStrength(req.Strength)
	ov, ou, okOff := doseStrength(off.UnitStrength)
	if !okReq || !okOff || ru != ou {
		return false, 0
	}
	ratio := rv / ov
	n := math.Round(ratio)
	if math.Abs(ratio-n) > 1e-9 || n < 1 || n > maxDoseUnitsPerDose {
		return false, 0
	}
	units := int(n)
	if off.Quantity < units*req.Quantity {
		return false, 0
	}
	return true, units
}

// EvaluateEquivalence runs the four equivalence checks and returns all of them in
// label order. lookup reads the government reference for the offered product.
func EvaluateEquivalence(req DoseRequest, off DoseOffer, lookup func(ingredient, strength, form string) (GovProduct, bool)) DoseCheck {
	dc := DoseCheck{
		Requested: fmt.Sprintf("%s × %d", req.Strength, req.Quantity),
		Offered:   fmt.Sprintf("%s × %d", off.UnitStrength, off.Quantity),
	}

	reqIng := normDoseIngredient(req.Ingredient)
	ingOK := reqIng != "" && reqIng == normDoseIngredient(off.Ingredient)

	reqForm := normDoseForm(req.DosageForm)
	formOK := reqForm != "" && reqForm == normDoseForm(off.DosageForm)

	totalOK, units := doseTotal(req, off)
	if totalOK {
		dc.UnitsPerDose = units
	}

	govOK := false
	if lookup != nil {
		if p, ok := lookup(off.Ingredient, off.UnitStrength, off.DosageForm); ok {
			govOK = true
			dc.GovCode = p.Code
		}
	}

	dc.Checks = []DoseCheckItem{
		{Label: DoseLabelIngredient, OK: ingOK},
		{Label: DoseLabelForm, OK: formOK},
		{Label: DoseLabelTotal, OK: totalOK},
		{Label: DoseLabelGov, OK: govOK},
	}
	dc.Accepted = ingOK && formOK && totalOK && govOK
	return dc
}
