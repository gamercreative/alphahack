package ai

// Text to speech for simulated calls. Providers:
//   say    macOS `say` (offline, free; Majed reads Arabic script, not Arabizi)
//   azure  Azure Speech REST (ar-LB Lebanese voices); needs AZURE_SPEECH_KEY + region
//   fake   a short silent WAV (tests, CI, Linux without a provider)

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Audio is one synthesized clip.
type Audio struct {
	Data        []byte
	ContentType string
	Voice       string
}

// Speech turns text into audio. lang is "ar", "en" or "fr"; speaker is "agent"
// or "pharmacy" (two voices so a simulated call sounds like two people).
type Speech interface {
	Name() string
	Synthesize(ctx context.Context, text, lang, speaker string) (Audio, error)
}

// SpeechOptions configures NewSpeech. Voices maps "<lang>_<speaker>" to a voice
// name; missing keys use the provider defaults below.
type SpeechOptions struct {
	Provider    string // say | azure | fake
	AzureKey    string
	AzureRegion string
	Voices      map[string]string
	HTTPClient  *http.Client
}

var sayVoices = map[string]string{
	"ar_agent": "Majed", "ar_pharmacy": "Majed",
	"en_agent": "Samantha", "en_pharmacy": "Eddy (English (US))",
	"fr_agent": "Thomas", "fr_pharmacy": "Jacques",
}

var azureVoices = map[string]string{
	"ar_agent": "ar-LB-LaylaNeural", "ar_pharmacy": "ar-LB-RamiNeural",
	"en_agent": "en-US-AvaNeural", "en_pharmacy": "en-US-AndrewNeural",
	"fr_agent": "fr-FR-DeniseNeural", "fr_pharmacy": "fr-FR-HenriNeural",
}

var azureLocales = map[string]string{"ar": "ar-LB", "en": "en-US", "fr": "fr-FR"}

// NewSpeech returns the provider named in o.Provider.
func NewSpeech(o SpeechOptions) (Speech, error) {
	switch o.Provider {
	case "say":
		return &saySpeech{voices: merge(sayVoices, o.Voices)}, nil
	case "azure":
		if o.AzureKey == "" || o.AzureRegion == "" {
			return nil, errors.New("azure speech needs AZURE_SPEECH_KEY and AZURE_SPEECH_REGION")
		}
		c := o.HTTPClient
		if c == nil {
			c = &http.Client{Timeout: 30 * time.Second}
		}
		return &azureSpeech{key: o.AzureKey, region: o.AzureRegion, voices: merge(azureVoices, o.Voices), http: c}, nil
	case "fake":
		return fakeSpeech{}, nil
	default:
		return nil, fmt.Errorf("unknown speech provider %q (use say, azure or fake)", o.Provider)
	}
}

func merge(base, over map[string]string) map[string]string {
	out := make(map[string]string, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

func voiceFor(voices map[string]string, lang, speaker string) string {
	if v, ok := voices[lang+"_"+speaker]; ok {
		return v
	}
	return voices["en_agent"]
}

// ---- say (macOS) -------------------------------------------------------------

type saySpeech struct{ voices map[string]string }

func (s *saySpeech) Name() string { return "say" }

func (s *saySpeech) Synthesize(ctx context.Context, text, lang, speaker string) (Audio, error) {
	voice := voiceFor(s.voices, lang, speaker)
	f, err := os.CreateTemp("", "speech-*.wav")
	if err != nil {
		return Audio{}, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	rate := "185"
	if speaker == "pharmacy" {
		rate = "170"
	}
	// Text goes in on stdin (-f -), never as an argument.
	cmd := exec.CommandContext(ctx, "say", "-v", voice, "-r", rate, "-o", path,
		"--file-format=WAVE", "--data-format=LEI16@22050", "-f", "-")
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Audio{}, fmt.Errorf("say (voice %q): %v: %s", voice, err, strings.TrimSpace(stderr.String()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Audio{}, err
	}
	return Audio{Data: data, ContentType: "audio/wav", Voice: voice}, nil
}

// ---- Azure Speech ------------------------------------------------------------

type azureSpeech struct {
	key, region string
	voices      map[string]string
	http        *http.Client
}

func (s *azureSpeech) Name() string { return "azure" }

func (s *azureSpeech) Synthesize(ctx context.Context, text, lang, speaker string) (Audio, error) {
	voice := voiceFor(s.voices, lang, speaker)
	locale := azureLocales[lang]
	if locale == "" {
		locale = "en-US"
	}
	var esc bytes.Buffer
	if err := xml.EscapeText(&esc, []byte(text)); err != nil {
		return Audio{}, err
	}
	var voiceAttr bytes.Buffer
	if err := xml.EscapeText(&voiceAttr, []byte(voice)); err != nil {
		return Audio{}, err
	}
	ssml := fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="%s"><voice name="%s">%s</voice></speak>`,
		locale, voiceAttr.String(), esc.String())

	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", s.region)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(ssml))
	if err != nil {
		return Audio{}, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", s.key)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-24khz-48kbitrate-mono-mp3")
	req.Header.Set("User-Agent", "alphahack-backend")
	resp, err := s.http.Do(req)
	if err != nil {
		return Audio{}, fmt.Errorf("azure speech: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return Audio{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Audio{}, fmt.Errorf("azure speech: status %d", resp.StatusCode)
	}
	return Audio{Data: body, ContentType: "audio/mpeg", Voice: voice}, nil
}

// ---- fake --------------------------------------------------------------------

type fakeSpeech struct{}

func (fakeSpeech) Name() string { return "fake" }

func (fakeSpeech) Synthesize(_ context.Context, _, lang, speaker string) (Audio, error) {
	return Audio{Data: silentWAV(8000, 2400), ContentType: "audio/wav", Voice: "fake-" + lang + "-" + speaker}, nil
}

// silentWAV builds a mono 16-bit PCM WAV of n silent samples.
func silentWAV(rate, n int) []byte {
	var b bytes.Buffer
	dataLen := n * 2
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+dataLen))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(dataLen))
	b.Write(make([]byte, dataLen))
	return b.Bytes()
}
