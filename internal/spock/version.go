// internal/spock/version.go
package spock

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// spockMajorVersion queries conn directly for the Spock version actually
// installed there, rather than trusting any declared/spec version — the
// running image is the only source of truth, and every SQL-shape decision
// gated on Spock version needs to match what's really deployed.
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
