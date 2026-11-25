// internal/spock/names.go
package spock

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// slotPair identifies a provider→subscriber subscription.
type slotPair struct {
	provider   string
	subscriber string
}

// SlotNames holds the replication slot name Spock generates for each
// provider→subscriber subscription in the config. Spock gives the subscriber's
// replication origin the same name.
//
// Spock shortens and rewrites long names (see gen_slot_name() in Spock), so
// the names come from Spock itself rather than being rebuilt here.
type SlotNames map[slotPair]string

// slot returns the slot name for a provider→subscriber pair. Every pair of
// configured nodes is loaded up front, so a missing pair is a programming
// error.
func (n SlotNames) slot(provider, subscriber string) string {
	name, ok := n[slotPair{provider, subscriber}]
	if !ok {
		panic(fmt.Sprintf("no spock slot name loaded for %s→%s", provider, subscriber))
	}
	return name
}

// LoadSlotNames asks Spock for the slot name of every provider→subscriber pair
// in the config. spock.spock_gen_slot_name() depends only on its arguments, so
// any node that has the Spock extension answers for every pair.
func LoadSlotNames(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) (SlotNames, error) {
	var providers, subscribers, subNames []string
	for _, src := range cfg.Nodes {
		for _, dst := range cfg.Nodes {
			if src.Name == dst.Name {
				continue
			}
			providers = append(providers, src.Name)
			subscribers = append(subscribers, dst.Name)
			subNames = append(subNames, spockSubName(src.Name, dst.Name))
		}
	}

	names := make(SlotNames, len(providers))
	if len(providers) == 0 {
		return names, nil
	}

	conn, err := slotNameConn(ctx, cfg, conns)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, `
		SELECT p, s, spock.spock_gen_slot_name($1::name, p::name, sub::name)::text
		  FROM unnest($2::text[], $3::text[], $4::text[]) AS t(p, s, sub)`,
		cfg.DBName, providers, subscribers, subNames)
	if err != nil {
		return nil, fmt.Errorf("get spock slot names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var provider, subscriber, name string
		if err := rows.Scan(&provider, &subscriber, &name); err != nil {
			return nil, fmt.Errorf("read spock slot name: %w", err)
		}
		names[slotPair{provider, subscriber}] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get spock slot names: %w", err)
	}
	if len(names) != len(providers) {
		return nil, fmt.Errorf("got %d spock slot names, want %d", len(names), len(providers))
	}
	return names, nil
}

// slotNameConn returns the connection of the first node that has
// spock.spock_gen_slot_name().
func slotNameConn(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) (*pgxpool.Pool, error) {
	for _, node := range cfg.Nodes {
		var hasFunc bool
		err := conns[node.Name].QueryRow(ctx,
			"SELECT to_regprocedure('spock.spock_gen_slot_name(name,name,name)') IS NOT NULL",
		).Scan(&hasFunc)
		if err != nil {
			return nil, fmt.Errorf("look for spock.spock_gen_slot_name on %s: %w", node.Name, err)
		}
		if hasFunc {
			return conns[node.Name], nil
		}
	}
	return nil, fmt.Errorf("spock.spock_gen_slot_name not found on any node; is the spock extension installed in database %q?", cfg.DBName)
}

// genSlotName asks Spock on conn for the slot name of a single
// provider→subscriber pair. It is used for pairs outside the config, such as
// subscriptions from a removed node.
func genSlotName(ctx context.Context, conn *pgxpool.Pool, dbName, provider, subscriber string) (string, error) {
	var name string
	err := conn.QueryRow(ctx,
		"SELECT spock.spock_gen_slot_name($1::name, $2::name, $3::name)::text",
		dbName, provider, spockSubName(provider, subscriber),
	).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("get spock slot name for %s→%s: %w", provider, subscriber, err)
	}
	return name, nil
}

// spockSubName returns the canonical Spock subscription name for a
// src→dst pair. Dashes are replaced with underscores to match PostgreSQL
// identifier conventions.
func spockSubName(srcName, dstName string) string {
	return strings.ReplaceAll(
		fmt.Sprintf("sub_%s_%s", srcName, dstName),
		"-", "_",
	)
}
