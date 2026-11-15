package config

import "testing"

func TestUploadDir(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"default", "", "./uploads"},
		{"blank is default", "   ", "./uploads"},
		{"set", "/var/lib/wassit/uploads", "/var/lib/wassit/uploads"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := load(mapEnv(baseEnv(map[string]string{"UPLOAD_DIR": tc.value})), noFile(t))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.UploadDir != tc.want {
				t.Fatalf("UploadDir = %q, want %q", got.UploadDir, tc.want)
			}
		})
	}
}
