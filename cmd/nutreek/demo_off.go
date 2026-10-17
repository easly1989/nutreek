//go:build !demo

package main

import (
	"context"
	"log/slog"

	"github.com/alexedwards/scs/v2"

	"github.com/amon2126/nutreek/internal/app"
	"github.com/amon2126/nutreek/internal/db"
)

// applyDemo does nothing in the normal build: the demo is built with -tags demo.
func applyDemo(context.Context, *db.Store, *scs.SessionManager, *app.App, *slog.Logger) error {
	return nil
}
