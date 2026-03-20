// internal/pg/pg.go
package pg

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPort    = 5432
	connectTimeout = 3 * time.Second
	defaultCertPath = "/certificates/admin/tls.crt"
	defaultKeyPath  = "/certificates/admin/tls.key"
)

// connectHost returns the host to use for connectivity checks.
// Uses internalHostname if set, otherwise falls back to hostname.
func connectHost(hostname, internalHostname string) string {
	if internalHostname != "" {
		return internalHostname
	}
	return hostname
}

// buildConnConfig creates a pgx connection config with TLS client certificates.
func buildConnConfig(host, dbName, user, certPath, keyPath string) (*pgx.ConnConfig, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load TLS client cert: %w", err)
	}

	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s sslmode=require connect_timeout=%d",
		host, defaultPort, dbName, user, int(connectTimeout.Seconds()),
	)
	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}

	cfg.TLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // matches Python sslmode=require (no server CA verification)
		MinVersion:         tls.VersionTLS12,
	}

	return cfg, nil
}

// Connect creates a new pgx connection to the given host.
func Connect(ctx context.Context, host, dbName, user string) (*pgx.Conn, error) {
	cfg, err := buildConnConfig(host, dbName, user, defaultCertPath, defaultKeyPath)
	if err != nil {
		return nil, err
	}
	return pgx.ConnectConfig(ctx, cfg)
}

// ConnectPool creates a new pgxpool connection pool to the given host.
// The pool is safe for concurrent use from multiple goroutines.
func ConnectPool(ctx context.Context, host, dbName, user string) (*pgxpool.Pool, error) {
	connCfg, err := buildConnConfig(host, dbName, user, defaultCertPath, defaultKeyPath)
	if err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(connCfg.ConnString())
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolCfg.ConnConfig = connCfg
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

// WaitReady polls until PostgreSQL accepts connections on the node.
// Uses internalHostname for the check, falls back to hostname.
func WaitReady(ctx context.Context, hostname, internalHostname, dbName, user string) error {
	host := connectHost(hostname, internalHostname)
	for {
		conn, err := Connect(ctx, host, dbName, user)
		if err == nil {
			conn.Close(ctx)
			slog.Info("node accepting connections", "hostname", hostname)
			return nil
		}
		slog.Info("waiting for node", "hostname", hostname, "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s: %w", hostname, ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
}
