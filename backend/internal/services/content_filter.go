package services

// Content filter on agent outbound text (CONTRACT rule 1: the agent never
// recommends or substitutes). Pure functions plus the EnquiryService hook used
// by Notify and RequestClarification. Staff text is never filtered.

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"alphahack/internal/queries"
)

// EvContentFiltered is logged (actor system/rules) when agent text is blocked.
const EvContentFiltered = "content_filtered"

// Latin-script phrases match on word boundaries; when the phrase ends in a word
// of 6+ letters the end may run on (so "substitute" also catches "substituted").
// Arabic phrases match anywhere because Arabic attaches prefixes such as و and
// ب to the word.
var (
	recommendPhrasesLatin = []string{
		// English
		"i recommend", "we recommend", "you should take", "instead take", "take instead",
		"try taking", "switch to", "substitute", "replace it with", "better to take",
		// French
		"je recommande", "à la place", "remplacer par", "prenez plutôt",
		// Arabizi
		"jarreb", "khod badal", "badal", "a7san tekhod",
	}
	recommendPhrasesArabic = []string{"استبدل", "أنصح", "بدلاً", "بدل", "جرّب", "جرب"} // longest match first
)

// CheckOutboundText reports whether agent text may be sent. It blocks a
// recommendation phrase (EN/FR/Arabizi/AR) or a catalogue medicine name (brand
// or active ingredient; combined ingredients split on "/") that is not in
// allowedNames. Matching is case-insensitive and ignores zero-width characters,
// Arabic diacritics and extra whitespace. reason teaches what to do instead.
func CheckOutboundText(text string, allowedNames []string, catalogueNames []string) (ok bool, reason string) {
	t := normalizeFilterText(text)
	if t == "" {
		return true, ""
	}
	for _, p := range recommendPhrasesLatin {
		if latinPhraseIn(t, normalizeFilterText(p)) {
			return false, phraseReason(p)
		}
	}
	for _, p := range recommendPhrasesArabic {
		if strings.Contains(t, normalizeFilterText(p)) {
			return false, phraseReason(p)
		}
	}

	allowed := map[string]bool{}
	for _, n := range allowedNames {
		n = normalizeFilterText(n)
		if n == "" {
			continue
		}
		allowed[n] = true
		for _, w := range filterWords(n) {
			allowed[w] = true
		}
	}
	for _, raw := range catalogueNames {
		for _, part := range strings.Split(raw, "/") {
			name := normalizeFilterText(part)
			if name == "" || isAllowedName(name, allowed) {
				continue
			}
			if containsWord(t, name) {
				return false, fmt.Sprintf("Blocked by the content filter: the text names %q, which is not the requested "+
					"medicine and was not offered by a pharmacy on this enquiry. Relay only the requested medicine and "+
					"what the pharmacies offered; never mention or suggest another medicine.", name)
			}
		}
	}
	return true, ""
}

func phraseReason(p string) string {
	return fmt.Sprintf("Blocked by the content filter: %q reads as a recommendation or substitution, and Wassit never "+
		"recommends or substitutes medicines. Rewrite the text to relay only the requested medicine and what the "+
		"pharmacies offered, in plain facts; the pharmacist decides.", p)
}

// isAllowedName: the whole name is allowed, or every word of it is.
func isAllowedName(name string, allowed map[string]bool) bool {
	if allowed[name] {
		return true
	}
	words := filterWords(name)
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if !allowed[w] {
			return false
		}
	}
	return true
}

// normalizeFilterText lowercases, drops zero-width / bidi / soft-hyphen
// characters and Arabic diacritics and tatweel, and collapses whitespace.
func normalizeFilterText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 0x200B && r <= 0x200F, r == 0x2060, r == 0xFEFF, r == 0x00AD, r == 0x061C:
			continue // zero-width, LRM/RLM, word joiner, BOM, soft hyphen, Arabic letter mark
		case (r >= 0x064B && r <= 0x0652) || r == 0x0670 || r == 0x0640:
			continue // Arabic diacritics (tashkeel), superscript alef, tatweel
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// filterWords splits s into runs of letters and digits.
func filterWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !isWordRune(r) })
}

// containsWord reports whether phrase occurs in t with a non-word character
// (or the text edge) on both sides.
func containsWord(t, phrase string) bool { return findBounded(t, phrase, true) }

// latinPhraseIn needs a boundary before the phrase, and after it unless the
// phrase's last word is 6+ letters long (then inflected endings still match).
func latinPhraseIn(t, phrase string) bool {
	words := filterWords(phrase)
	needEnd := len(words) == 0 || len([]rune(words[len(words)-1])) < 6
	return findBounded(t, phrase, needEnd)
}

func findBounded(t, phrase string, needEnd bool) bool {
	if phrase == "" {
		return false
	}
	for from := 0; from <= len(t)-len(phrase); {
		i := strings.Index(t[from:], phrase)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(phrase)
		if boundaryBefore(t, start) && (!needEnd || boundaryAfter(t, end)) {
			return true
		}
		from = start + 1
	}
	return false
}

func boundaryBefore(t string, i int) bool {
	if i == 0 {
		return true
	}
	r := []rune(t[:i])
	return !isWordRune(r[len(r)-1])
}

func boundaryAfter(t string, i int) bool {
	if i >= len(t) {
		return true
	}
	for _, r := range t[i:] {
		return !isWordRune(r)
	}
	return true
}

// filterAgentText runs CheckOutboundText for agent text on enquiry e inside the
// caller's transaction. When the text is blocked it logs a content_filtered
// event (actor system/rules) on q and returns the teaching reason; the caller
// must then commit (so the event is kept), queue nothing and return
// ContentFiltered(reason).
func (s *EnquiryService) filterAgentText(ctx context.Context, q queries.Querier, e queries.Enquiry, text string) (string, error) {
	// Names the agent may use: the requested medicine, the requester's own words
	// (raw request and clarification answer) and what the pharmacies replied.
	allowed := []string{e.RawRequest, e.ClarificationAnswer}
	if e.MedicationID != nil && *e.MedicationID != "" {
		m, err := q.GetMedication(ctx, *e.MedicationID)
		if err != nil {
			return "", FromStore(err, "medication")
		}
		allowed = append(allowed, m.BrandName)
		allowed = append(allowed, strings.Split(m.ActiveIngredient, "/")...)
	}
	targets, err := q.ListEnquiryTargets(ctx, e.ID)
	if err != nil {
		return "", FromStore(err, "enquiry")
	}
	for _, t := range targets {
		allowed = append(allowed, filterWords(normalizeFilterText(t.ReplyText))...)
	}
	meds, err := q.ListMedications(ctx, MaxLimit*5, 0)
	if err != nil {
		return "", FromStore(err, "medication")
	}
	catalogue := make([]string, 0, 2*len(meds))
	for _, m := range meds {
		catalogue = append(catalogue, m.BrandName, m.ActiveIngredient)
	}
	ok, reason := CheckOutboundText(text, allowed, catalogue)
	if ok {
		return "", nil
	}
	if _, err := s.logEvent(ctx, q, e.ID, RulesActor, EvContentFiltered,
		map[string]any{"text": text, "reason": reason}, nil); err != nil {
		return "", err
	}
	return reason, nil
}
