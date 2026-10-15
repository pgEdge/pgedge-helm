// internal/spock/version.go
package spock

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// spockMajorVersion queries conn for the Spock version actually installed, since the running image is the only source of truth.
func spockMajorVersion(ctx context.Context, conn *pgxpool.Pool) (uint64, error) {
	var versionStr string
	if err := conn.QueryRow(ctx, "SELECT spock.spock_version()").Scan(&versionStr); err != nil {
		return 0, fmt.Errorf("query spock version: %w", err)
	}
	major, err := parseSpockMajorVersion(versionStr)
	if err != nil {
		return 0, fmt.Errorf("parse spock version %q: %w", versionStr, err)
	}
	return major, nil
}

// availableSpockMajorVersion is like spockMajorVersion, but works even if the spock extension doesn't exist in this database yet.
func availableSpockMajorVersion(ctx context.Context, conn *pgxpool.Pool) (uint64, error) {
	var versionStr string
	err := conn.QueryRow(ctx,
		"SELECT default_version FROM pg_available_extensions WHERE name = 'spock'",
	).Scan(&versionStr)
	if err != nil {
		return 0, fmt.Errorf("query available spock version: %w", err)
	}
	major, err := parseSpockMajorVersion(versionStr)
	if err != nil {
		return 0, fmt.Errorf("parse spock version %q: %w", versionStr, err)
	}
	return major, nil
}

// parseSpockMajorVersion extracts the leading major version component from
// a Spock version string (e.g. "5.0.6" -> 5, "6.0.0-beta1" -> 6).
func parseSpockMajorVersion(versionStr string) (uint64, error) {
	major, _, _ := strings.Cut(versionStr, ".")
	major, _, _ = strings.Cut(major, "-")
	n, err := strconv.ParseUint(major, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid major version component %q: %w", major, err)
	}
	return n, nil
}

// pgMajorVersion queries conn for the running Postgres server's major version.
func pgMajorVersion(ctx context.Context, conn *pgxpool.Pool) (uint64, error) {
	var versionNum int64
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')::integer").Scan(&versionNum); err != nil {
		return 0, fmt.Errorf("query postgres version: %w", err)
	}
	return uint64(versionNum) / 10000, nil
}

// needsNativeFailoverSlots reports whether conn needs the FAILOVER flag on slot creation: required on PostgreSQL 18+, where Spock 6 drops its own failover-slot worker entirely, and used on 17+ as well for consistency; see docs/usage/standby.md.
func needsNativeFailoverSlots(ctx context.Context, conn *pgxpool.Pool) (bool, error) {
	spockMajor, err := spockMajorVersion(ctx, conn)
	if err != nil {
		return false, err
	}
	pgMajor, err := pgMajorVersion(ctx, conn)
	if err != nil {
		return false, err
	}
	return spockMajor >= 6 && pgMajor >= 17, nil
}
