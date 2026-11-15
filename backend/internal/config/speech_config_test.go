package config

import (
	"runtime"
	"strings"
	"testing"
)

// nativeSpeech is the speech default without an Azure key on this OS.
func nativeSpeech() string {
	if runtime.GOOS == "darwin" {
		return "say"
	}
	return "fake"
}

func TestSpeechProviderDefault(t *testing.T) {
	native := nativeSpeech()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no key uses the platform default", nil, native},
		{"azure when key set", map[string]string{"AZURE_SPEECH_KEY": "k", "AZURE_SPEECH_REGION": "westeurope"}, "azure"},
		{"explicit wins over key", map[string]string{"AZURE_SPEECH_KEY": "k", "AZURE_SPEECH_REGION": "r", "SPEECH_PROVIDER": "fake"}, "fake"},
		{"off is allowed", map[string]string{"SPEECH_PROVIDER": "off"}, "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := load(mapEnv(baseEnv(tt.env)), noFile(t))
			if err != nil {
				t.Fatal(err)
			}
			if got.SpeechProvider != tt.want {
				t.Fatalf("SpeechProvider = %q, want %q", got.SpeechProvider, tt.want)
			}
		})
	}
}

func TestSpeechValidation(t *testing.T) {
	tests := map[string]map[string]string{
		"azure without region": {"SPEECH_PROVIDER": "azure", "AZURE_SPEECH_KEY": "k"},
		"key without region":   {"AZURE_SPEECH_KEY": "k"},
		"azure without key":    {"SPEECH_PROVIDER": "azure", "AZURE_SPEECH_REGION": "r"},
		"unknown provider":     {"SPEECH_PROVIDER": "espeak"},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := load(mapEnv(baseEnv(env)), noFile(t))
			if err == nil || !strings.Contains(err.Error(), "SPEECH_PROVIDER") {
				t.Fatalf("want SPEECH_PROVIDER error, got %v", err)
			}
		})
	}
}

func TestSpeechVoices(t *testing.T) {
	got, err := load(mapEnv(baseEnv(map[string]string{
		"SPEECH_PROVIDER": "fake", "SPEECH_VOICE_AR_PHARMACY": "ar-LB-RamiNeural",
	})), noFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.SpeechVoices["ar_pharmacy"] != "ar-LB-RamiNeural" || len(got.SpeechVoices) != 1 {
		t.Fatalf("SpeechVoices = %v", got.SpeechVoices)
	}
}
