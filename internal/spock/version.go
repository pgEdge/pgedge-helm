// internal/spock/version.go
package spock

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// availableSpockMajor returns the major version of the Spock binaries the
// node runs (the extension's default_version), which works whether or not
// the extension has been created in the database yet.
func availableSpockMajor(ctx context.Context, conn *pgxpool.Pool) (int, error) {
	var version string
	err := conn.QueryRow(ctx,
		"SELECT default_version FROM pg_available_extensions WHERE name = 'spock'",
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query available spock version: %w", err)
	}
	return parseSpockMajor(version)
}

// parseSpockMajor returns the major component of a Spock version string,
// e.g. "5.0.14" -> 5, "6.0.0" -> 6, "6.0.0-beta1" -> 6.
func parseSpockMajor(version string) (int, error) {
	major, _, _ := strings.Cut(version, ".")
	major, _, _ = strings.Cut(major, "-")
	n, err := strconv.Atoi(major)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid spock version %q", version)
	}
	return n, nil
}

// serverMajor returns the PostgreSQL major version of the node.
func serverMajor(ctx context.Context, conn *pgxpool.Pool) (int, error) {
	var num int
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&num); err != nil {
		return 0, fmt.Errorf("query postgres version: %w", err)
	}
	return num / 10000, nil
}
