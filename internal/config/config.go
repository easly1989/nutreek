// Package config carica la configurazione runtime da variabili d'ambiente.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
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
	OFFDumpURL        string
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

	// PremiumEnabled attiva il gating premium. Se false HasFeature() restituisce
	// sempre true e le feature premium sono accessibili a tutti.
	PremiumEnabled bool
}

// Load legge la configurazione dalle env var. Valida i vincoli di prod
// (p.es. SessionKey obbligatoria) e genera una chiave ephemeral in dev.
func Load() (*Config, error) {
	env := Env(getenv("NUTREEK_ENV", string(EnvDev)))
	if env != EnvDev && env != EnvProd {
		return nil, fmt.Errorf("NUTREEK_ENV: valore non valido %q (attesi: dev, prod)", env)
	}

	reconcileInterval, err := time.ParseDuration(getenv("NUTREEK_RECONCILE_INTERVAL", "24h"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_RECONCILE_INTERVAL: %w", err)
	}

	reconcileEnabled, err := strconv.ParseBool(getenv("NUTREEK_RECONCILE_ENABLED", "true"))
	if err != nil {
		return nil, fmt.Errorf("NUTREEK_RECONCILE_ENABLED: %w", err)
	}

	offSyncInterval, err := time.ParseDuration(getenv("NUTREEK_OFF_SYNC_INTERVAL", "168h"))
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

	cfg := &Config{
		Addr:              getenv("NUTREEK_ADDR", ":8080"),
		DBPath:            getenv("NUTREEK_DB_PATH", "./data/nutreek.db"),
		Env:               env,
		OFFBaseURL:        getenv("NUTREEK_OFF_BASE_URL", "https://world.openfoodfacts.org"),
		OFFDumpURL:        getenv("NUTREEK_OFF_DUMP_URL", "https://static.openfoodfacts.org/data/openfoodfacts-products.jsonl.gz"),
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
		return nil, errors.New("NUTREEK_SESSION_KEY e' obbligatoria in prod (es. openssl rand -hex 32)")
	default:
		// dev fallback: chiave ephemeral rigenerata a ogni avvio
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generazione session key ephemeral: %w", err)
		}
		cfg.SessionKey = key
		slog.Warn("NUTREEK_SESSION_KEY non impostata: uso chiave ephemeral (sessioni non sopravvivono al restart)")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
