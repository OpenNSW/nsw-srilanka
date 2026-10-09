package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/OpenNSW/core/refid"
	refidpg "github.com/OpenNSW/core/refid/store/postgres"
	"gorm.io/gorm"
)

// initRefIDs builds the registry that REFID_GENERATOR steps and payment
// references generate from. NewRegistry compiles every configured format up
// front, so a malformed refid section in config.yaml stops the boot.
//
// The stores share db's connection pool, and their tables come from migration
// 000017.
func initRefIDs(cfg refid.Config, db *gorm.DB) (refid.Registry, error) {
	if len(cfg.Issuers) == 0 {
		slog.Info("reference ID generation not configured; REFID_GENERATOR " +
			"steps and payments fail until config.yaml defines refid.issuers")
		return disabledRefIDs{}, nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB for refid stores: %w", err)
	}
	sequences, err := refidpg.NewSequence(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("failed to create refid sequence store: %w", err)
	}
	randoms, err := refidpg.NewRandom(sqlDB)
	if err != nil {
		return nil, fmt.Errorf("failed to create refid random store: %w", err)
	}

	registry, err := refid.NewRegistry(cfg,
		refid.WithSequenceStore(sequences),
		refid.WithRandomStore(randoms),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid refid config: %w", err)
	}
	slog.Info("reference ID generation configured", "issuers", len(cfg.Issuers))
	return registry, nil
}

// disabledRefIDs is the registry for a deployment with no refid section. Every
// Generate fails, so a template using the plugin is a loud misconfiguration.
type disabledRefIDs struct{}

// Generate wraps ErrUnknownIssuer so callers classifying refid errors need no
// special case; the message names the cause, which the sentinel alone doesn't.
func (disabledRefIDs) Generate(_ context.Context, issuer, idType string, _ map[string]string) (string, error) {
	return "", fmt.Errorf("%w: config.yaml has no refid issuers configured, so (%q, %q) cannot be generated",
		refid.ErrUnknownIssuer, issuer, idType)
}
