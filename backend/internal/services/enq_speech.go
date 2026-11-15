package services

// Simulated calls: text to speech for outbound messages and pharmacist replies.
// The audio is a demo stand-in for a phone call; nothing is dialled.

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"alphahack/internal/ai"
)

const maxSpeechChars = 600

// SpeechInput is the body of POST /api/v1/speech.
type SpeechInput struct {
	Text    string `json:"text"`
	Lang    string `json:"lang"`    // ar | en | fr | "" (detect)
	Speaker string `json:"speaker"` // agent | pharmacy | "" (agent)
}

// FakeSpeech is the offline provider (a short silent WAV) for tests outside
// internal/ai, e.g. handler tests, which may not import internal/ai.
func FakeSpeech() ai.Speech {
	sp, err := ai.NewSpeech(ai.SpeechOptions{Provider: "fake"})
	if err != nil {
		panic(err) // "fake" never fails
	}
	return sp
}

// Speak synthesizes one line of a simulated call for staff or the agent.
func (s *EnquiryService) Speak(ctx context.Context, _ Actor, in SpeechInput) (ai.Audio, error) {
	if s.cfg.Speech == nil {
		return ai.Audio{}, Conflict("speech is off; set SPEECH_PROVIDER to say, azure or fake and restart")
	}
	text := strings.TrimSpace(in.Text)
	if n := utf8.RuneCountInString(text); n < 1 || n > maxSpeechChars {
		return ai.Audio{}, Invalid("text must be 1 to %d characters", maxSpeechChars)
	}
	lang := strings.ToLower(strings.TrimSpace(in.Lang))
	switch lang {
	case "":
		lang = DetectLang(text)
	case "ar", "en", "fr":
	default:
		return ai.Audio{}, Invalid("lang must be ar, en, fr or empty to detect")
	}
	speaker := strings.ToLower(strings.TrimSpace(in.Speaker))
	switch speaker {
	case "":
		speaker = "agent"
	case "agent", "pharmacy":
	default:
		return ai.Audio{}, Invalid("speaker must be agent or pharmacy")
	}
	audio, err := s.cfg.Speech.Synthesize(ctx, text, lang, speaker)
	if err != nil {
		if ctx.Err() != nil {
			return ai.Audio{}, ctx.Err()
		}
		return ai.Audio{}, &Error{Code: CodeUpstream, Message: "the speech provider request failed", Err: err}
	}
	return audio, nil
}

var frenchHints = []string{"bonjour", "merci", "avez-vous", "vous", "comprimé", "boîte", "disponible", "pharmacie"}

// DetectLang guesses ar (any Arabic script), fr (French words or accents) or en.
// Arabizi (Arabic in Latin letters) comes out as en; pass lang explicitly for it.
func DetectLang(text string) string {
	for _, r := range text {
		if unicode.In(r, unicode.Arabic) {
			return "ar"
		}
	}
	low := strings.ToLower(text)
	if strings.ContainsAny(low, "éèêàçùôî") {
		return "fr"
	}
	for _, w := range frenchHints {
		if strings.Contains(low, w) {
			return "fr"
		}
	}
	return "en"
}
