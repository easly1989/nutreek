package config

import (
	"strings"
	"testing"
)

func TestLoadOIDC(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "off by default", env: nil},
		{name: "complete", env: map[string]string{"NUTREEK_BASE_URL": "https://n.example.com/", "NUTREEK_OIDC_ISSUER": "https://auth.example.com",
			"NUTREEK_OIDC_CLIENT_ID": "nutreek", "NUTREEK_OIDC_CLIENT_SECRET": "s"}},
		{name: "issuer without a base address", env: map[string]string{"NUTREEK_OIDC_ISSUER": "https://auth.example.com",
			"NUTREEK_OIDC_CLIENT_ID": "nutreek", "NUTREEK_OIDC_CLIENT_SECRET": "s"}, wantErr: "NUTREEK_BASE_URL"},
		{name: "issuer without a secret", env: map[string]string{"NUTREEK_BASE_URL": "https://n.example.com", "NUTREEK_OIDC_ISSUER": "https://auth.example.com",
			"NUTREEK_OIDC_CLIENT_ID": "nutreek"}, wantErr: "CLIENT_SECRET"},
		{name: "base address without a scheme", env: map[string]string{"NUTREEK_BASE_URL": "n.example.com"}, wantErr: "NUTREEK_BASE_URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"NUTREEK_BASE_URL", "NUTREEK_OIDC_ISSUER", "NUTREEK_OIDC_CLIENT_ID", "NUTREEK_OIDC_CLIENT_SECRET"} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one with %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "complete" && cfg.BaseURL != "https://n.example.com" {
				t.Fatalf("the trailing slash should go: %q", cfg.BaseURL)
			}
		})
	}
}
