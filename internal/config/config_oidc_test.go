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

func TestLoadSMTP(t *testing.T) {
	for _, k := range []string{"NUTREEK_BASE_URL", "NUTREEK_SMTP_HOST", "NUTREEK_SMTP_FROM", "NUTREEK_SMTP_PORT"} {
		t.Setenv(k, "")
	}
	if cfg, err := Load(); err != nil || cfg.SMTPHost != "" {
		t.Fatalf("off by default: %v", err)
	}
	t.Setenv("NUTREEK_SMTP_HOST", "smtp.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NUTREEK_BASE_URL") {
		t.Fatalf("a host without a base address: %v", err)
	}
	t.Setenv("NUTREEK_BASE_URL", "https://n.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NUTREEK_SMTP_FROM") {
		t.Fatalf("a host without a sender: %v", err)
	}
	t.Setenv("NUTREEK_SMTP_FROM", "Nutreek <n@example.com>")
	cfg, err := Load()
	if err != nil || cfg.SMTPPort != 587 || cfg.SMTPTLS != "starttls" {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
	t.Setenv("NUTREEK_SMTP_PORT", "mail")
	if _, err := Load(); err == nil {
		t.Fatal("a port that is not a number should fail")
	}
}

func TestParseProxies(t *testing.T) {
	list, err := ParseProxies("10.0.0.5, 172.18.0.0/16 ,fd00::1")
	if err != nil || len(list) != 3 || list[0].String() != "10.0.0.5/32" || list[1].String() != "172.18.0.0/16" || list[2].String() != "fd00::1/128" {
		t.Fatalf("got %v, %v", list, err)
	}
	if list, err := ParseProxies("::ffff:10.0.0.0/104"); err != nil || len(list) != 1 || list[0].String() != "10.0.0.0/8" {
		t.Fatalf("a mapped range is unmapped: %v, %v", list, err)
	}
	if list, err := ParseProxies("none"); err != nil || len(list) != 0 {
		t.Fatalf("none: %v, %v", list, err)
	}
	for _, bad := range []string{"not-an-address", "10.0.0.0/99", "10.0.0.1/x", "0.0.0.0/0", "::/0", "none,10.0.0.5", "::ffff:0.0.0.0/0"} {
		if _, err := ParseProxies(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	t.Setenv("NUTREEK_TRUSTED_PROXIES", "")
	if cfg, err := Load(); err != nil || cfg.TrustedProxiesSet {
		t.Fatalf("not set: %v", err)
	}
	t.Setenv("NUTREEK_TRUSTED_PROXIES", "10.0.0.5")
	if cfg, err := Load(); err != nil || !cfg.TrustedProxiesSet || len(cfg.TrustedProxies) != 1 {
		t.Fatalf("set: %v", err)
	}
	t.Setenv("NUTREEK_TRUSTED_PROXIES", "nonsense")
	if _, err := Load(); err == nil {
		t.Fatal("a bad value should stop the start")
	}
}
