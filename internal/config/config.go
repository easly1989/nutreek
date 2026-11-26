// Package config loads the runtime configuration from environment variables.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	EnvDev  Env = "dev"
	EnvProd Env = "prod"
)

type Config struct {
	Addr              string
	DBPath            string
	Env               Env
	SessionKey        []byte
	OFFBaseURL        string
	OFFCSVURL         string
	OFFDeltaURL       string
	OFFSyncInterval   time.Duration
	OFFSyncEnabled    bool
	ReconcileInterval time.Duration
	ReconcileEnabled  bool
	AdminToken        string
	OwnerEmail        string

	// Backup SQLite automatico via VACUUM INTO.
	BackupEnabled  bool
	BackupDir      string
	BackupInterval time.Duration
	BackupKeep     int

	// PremiumEnabled turns premium gating on. When false HasFeature() always
	// returns true and the premium features are available to everyone.
	PremiumEnabled bool

	// UpdateCheck lets the server look for newer Nutreek releases once a day
	// (#167); false turns it off for the whole installation.
	UpdateCheck bool

	// SecretKey seals the tokens of the store plugins (#239); empty = plugins cannot be
	// connected. StoresAllowPrivate lets a plugin sit on a private address (a home server).
	SecretKey          string
	StoresAllowPrivate bool

	// BaseURL is the public address of this instance (no trailing slash): the
	// callback of the sign-in provider is built on it (#333).
	BaseURL string
	// OIDC* turn on "Sign in with <name>" (#333). IssuerURL empty = off.
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCName         string
	OIDCAutoCreate   bool

	// SMTP* turn on the emails (#334: password recovery). SMTPHost empty = no email is sent.
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPTLS      string // starttls (default), tls or none

	// TrustedProxies are the addresses whose X-Forwarded-For is believed (#346). TrustedProxiesSet is
	// false when the variable is not set: then loopback and private networks are trusted, as before.
	TrustedProxies    []netip.Prefix
	TrustedProxiesSet bool
}

// Load reads the configuration from the env vars. It validates the prod
// constraints (e.g. a required SessionKey) and generates an ephemeral key in dev.
func Load() (*Config, error) {
	env := Env(getenv("NUTREEK_ENV", string(EnvDev)))
	if env != EnvDev && env != EnvProd {
		return nil, fmt.Errorf("NUTREEK_ENV: invalid value %q (want: dev, prod)", env)
	}

	reconcileInterval, err := time.ParseDuration(getenv("NUTREEK_RECONCILE_INTERVAL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_RECONCILE_INTERVAL: %w", err)
	}

	reconcileEnabled, err := strconv.ParseBool(getenv("NUTREEK_RECONCILE_ENABLED", "true"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_RECONCILE_ENABLED: %w", err)
	}

	offSyncInterval, err := time.ParseDuration(getenv("NUTREEK_OFF_SYNC_INTERVAL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_OFF_SYNC_INTERVAL: %w", err)
	}

	offSyncEnabled, err := strconv.ParseBool(getenv("NUTREEK_OFF_SYNC_ENABLED", "true"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_OFF_SYNC_ENABLED: %w", err)
	}

	backupEnabled, err := strconv.ParseBool(getenv("NUTREEK_BACKUP_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_BACKUP_ENABLED: %w", err)
	}

	backupInterval, err := time.ParseDuration(getenv("NUTREEK_BACKUP_INTERVAL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_BACKUP_INTERVAL: %w", err)
	}

	backupKeep, err := strconv.Atoi(getenv("NUTREEK_BACKUP_KEEP", "7"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_BACKUP_KEEP: %w", err)
	}

	premiumEnabled, err := strconv.ParseBool(getenv("NUTREEK_PREMIUM_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_PREMIUM_ENABLED: %w", err)
	}

	updateCheck, err := strconv.ParseBool(getenv("NUTREEK_UPDATE_CHECK", "true"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_UPDATE_CHECK: %w", err)
	}

	storesAllowPrivate, err := strconv.ParseBool(getenv("NUTREEK_STORES_ALLOW_PRIVATE", "false"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_STORES_ALLOW_PRIVATE: %w", err)
	}
	secret := os.Getenv("NUTREEK_SECRET_KEY")
	if secret != "" && len(secret) < 16 {
		return nil, errors.New("NUTREEK_SECRET_KEY: at least 16 characters required (e.g. openssl rand -hex 32)")
	}

	oidcAutoCreate, err := strconv.ParseBool(getenv("NUTREEK_OIDC_AUTO_CREATE", "false"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_OIDC_AUTO_CREATE: %w", err)
	}
	smtpPort, err := strconv.Atoi(getenv("NUTREEK_SMTP_PORT", "587"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_SMTP_PORT: %w", err)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("NUTREEK_BASE_URL")), "/")
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, errors.New("NUTREEK_BASE_URL: an absolute http(s) address is required (e.g. https://nutreek.example.com)")
		}
	}

	cfg := &Config{
		Addr:              getenv("NUTREEK_ADDR", ":8080"),
		DBPath:            getenv("NUTREEK_DB_PATH", "./data/nutreek.db"),
		Env:               env,
		OFFBaseURL:        getenv("NUTREEK_OFF_BASE_URL", "https://world.openfoodfacts.org"),
		OFFCSVURL:         getenv("NUTREEK_OFF_CSV_URL", "https://static.openfoodfacts.org/data/en.openfoodfacts.org.products.csv.gz"),
		OFFDeltaURL:       getenv("NUTREEK_OFF_DELTA_URL", "https://static.openfoodfacts.org/data/delta/"),
		OFFSyncInterval:   offSyncInterval,
		OFFSyncEnabled:    offSyncEnabled,
		ReconcileInterval: reconcileInterval,
		ReconcileEnabled:  reconcileEnabled,
		AdminToken:        os.Getenv("NUTREEK_ADMIN_TOKEN"),
		OwnerEmail:        os.Getenv("NUTREEK_OWNER_EMAIL"),
		BackupEnabled:     backupEnabled,
		BackupDir:         getenv("NUTREEK_BACKUP_DIR", "./data/backups"),
		BackupInterval:    backupInterval,
		BackupKeep:        backupKeep,
		PremiumEnabled:    premiumEnabled,
		UpdateCheck:       updateCheck,

		SecretKey:          secret,
		StoresAllowPrivate: storesAllowPrivate,

		BaseURL:          baseURL,
		OIDCIssuer:       strings.TrimSpace(os.Getenv("NUTREEK_OIDC_ISSUER")),
		OIDCClientID:     strings.TrimSpace(os.Getenv("NUTREEK_OIDC_CLIENT_ID")),
		OIDCClientSecret: os.Getenv("NUTREEK_OIDC_CLIENT_SECRET"),
		OIDCName:         getenv("NUTREEK_OIDC_NAME", "Single sign-on"),
		OIDCAutoCreate:   oidcAutoCreate,

		SMTPHost:     strings.TrimSpace(os.Getenv("NUTREEK_SMTP_HOST")),
		SMTPPort:     smtpPort,
		SMTPUser:     os.Getenv("NUTREEK_SMTP_USER"),
		SMTPPassword: os.Getenv("NUTREEK_SMTP_PASSWORD"),
		SMTPFrom:     strings.TrimSpace(os.Getenv("NUTREEK_SMTP_FROM")),
		SMTPTLS:      strings.ToLower(getenv("NUTREEK_SMTP_TLS", "starttls")),
	}
	if raw, ok := os.LookupEnv("NUTREEK_TRUSTED_PROXIES"); ok && strings.TrimSpace(raw) != "" {
		list, err := ParseProxies(raw)
		if err != nil {
			return nil, fmt.Errorf("NUTREEK_TRUSTED_PROXIES: %w", err)
		}
		cfg.TrustedProxies, cfg.TrustedProxiesSet = list, true
	}
	if cfg.SMTPHost != "" {
		switch {
		case cfg.BaseURL == "":
			return nil, errors.New("NUTREEK_SMTP_HOST needs NUTREEK_BASE_URL (the links in the emails are built on it)")
		case cfg.SMTPFrom == "":
			return nil, errors.New("NUTREEK_SMTP_HOST needs NUTREEK_SMTP_FROM (the sender address)")
		}
	}
	if cfg.OIDCIssuer != "" {
		switch {
		case cfg.BaseURL == "":
			return nil, errors.New("NUTREEK_OIDC_ISSUER needs NUTREEK_BASE_URL (the callback address)")
		case cfg.OIDCClientID == "" || cfg.OIDCClientSecret == "":
			return nil, errors.New("NUTREEK_OIDC_ISSUER needs NUTREEK_OIDC_CLIENT_ID and NUTREEK_OIDC_CLIENT_SECRET")
		}
	}

	keyHex := os.Getenv("NUTREEK_SESSION_KEY")
	switch {
	case keyHex != "":
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("NUTREEK_SESSION_KEY: attesa stringa esadecimale: %w", err)
		}
		if len(key) < 32 {
			return nil, errors.New("NUTREEK_SESSION_KEY: almeno 32 byte (64 hex char) richiesti")
		}
		cfg.SessionKey = key
	case env == EnvProd:
		return nil, errors.New("NUTREEK_SESSION_KEY is required in prod (e.g. openssl rand -hex 32)")
	default:
		// dev fallback: an ephemeral key regenerated on every start
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generazione session key ephemeral: %w", err)
		}
		cfg.SessionKey = key
		slog.Warn("NUTREEK_SESSION_KEY not set: using an ephemeral key (sessions do not survive a restart)")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// ParseProxies reads a comma-separated list of addresses and ranges (10.0.0.5, 172.18.0.0/16,
// fd00::/8). "none" trusts no proxy at all.
func ParseProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			continue
		case strings.EqualFold(part, "none"):
			continue
		case strings.Contains(part, "/"):
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not a range", part)
			}
			out = append(out, p.Masked())
		default:
			a, err := netip.ParseAddr(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address", part)
			}
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
		}
	}
	return out, nil
}
