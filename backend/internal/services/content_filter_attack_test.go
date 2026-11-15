package services

// Attack cases for CheckOutboundText: tricks an agent could use to slip a
// recommendation or another medicine past the filter.

import "testing"

var (
	attackCatalogue = []string{"Augmentin", "Amoxicillin/Clavulanate", "Brufen", "Ibuprofen",
		"Panadol Advance", "Paracetamol", "Concor", "Bisoprolol"}
	attackAllowed = []string{"Augmentin", "Amoxicillin", "Clavulanate"}
)

func TestContentFilterAttackBlocked(t *testing.T) {
	for name, text := range map[string]string{
		"zero-width inside a name":       "Bru\u200bfen is in stock at Achrafieh",
		"zero-width as a space":          "you\u200bshould take it twice",
		"word joiner and BOM":            "I\u2060 recommend\ufeff it",
		"mixed case name":                "bRuFeN 400 is available",
		"mixed case phrase":              "We ReCoMmEnD the other one",
		"name glued to dose":             "Ibuprofen400mg is in stock",
		"possessive":                     "Brufen's box is ready",
		"short brand of a 2-word name":   "Panadol is available instead",
		"fullwidth letters":              "Ｂｒｕｆｅｎ is in stock",
		"cyrillic look-alike":            "Вrufen is in stock",
		"french without accent":          "prenez celui-ci a la place",
		"french with accent":             "à la place, prenez plutôt l'autre",
		"arabizi":                        "khod badal 3anna",
		"arabic recommend":               "أنصح بهذا الدواء",
		"arabic with tashkeel":           "بدلاً منه",
		"arabic split by zero-width":     "ب\u200bدل",
		"other medicine ingredient":      "paracetamol is in stock",
		"other ingredient with accent":   "paracétamol est disponible",
		"substitute inflected uppercase": "SUBSTITUTED with a generic",
		"nbsp and tabs":                  "you\u00a0should\ttake two",
	} {
		t.Run(name, func(t *testing.T) {
			if ok, reason := CheckOutboundText(text, attackAllowed, attackCatalogue); ok || reason == "" {
				t.Fatalf("%q passed the filter", text)
			}
		})
	}
}

func TestContentFilterAllowedNamesPass(t *testing.T) {
	for _, text := range []string{
		"Augmentin 1g x2 is available at Achrafieh, pickup before 18:00.",
		"AUGMENTIN available; amoxicillin/clavulanate 1000mg.",
		"Achrafieh has no stock. Baabda did not answer yet.",
		"لا يوجد أوغمنتين في الأشرفية",
		"Do you need the 1g tablets or the suspension?",
	} {
		if ok, reason := CheckOutboundText(text, attackAllowed, attackCatalogue); !ok {
			t.Errorf("%q blocked: %s", text, reason)
		}
	}
}

func TestContentFilterRelayOfferedNamePasses(t *testing.T) {
	// A pharmacy offered Brufen in its reply: relaying it as a fact is allowed.
	allowed := append(append([]string{}, attackAllowed...), filterWords(normalizeFilterText("ma fi, 3ande Brufen bas"))...)
	if ok, reason := CheckOutboundText("Achrafieh offers Brufen as an alternative; not a match.", allowed, attackCatalogue); !ok {
		t.Fatalf("relay blocked: %s", reason)
	}
}
