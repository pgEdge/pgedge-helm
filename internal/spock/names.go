// internal/spock/names.go
package spock

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pgEdge/pgedge-helm/internal/config"
)

// spockSlotName returns the replication slot name Spock generates for the
// provider→subscriber subscription. Spock uses the same name for the
// subscriber's replication origin.
func spockSlotName(dbName, providerName, subscriberName string) string {
	return genSlotName(dbName, providerName, spockSubName(providerName, subscriberName))
}

// genSlotName mirrors Spock's gen_slot_name(): "spk_<db>_<provider>_<sub>",
// where each part longer than 16 bytes is shortened, then every byte outside
// [a-z0-9_] becomes an underscore. The result is at most 51 bytes, so the
// 63-byte name limit never truncates it.
func genSlotName(dbName, providerNode, subName string) string {
	name := []byte("spk_" + shortenHash(dbName) + "_" + shortenHash(providerNode) + "_" + shortenHash(subName))
	for i, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' {
			name[i] = '_'
		}
	}
	return string(name)
}

// shortenHash mirrors Spock's shorten_hash(str, 16): strings longer than 16
// bytes become their first 8 bytes plus the first 7 hex digits of hash_any().
func shortenHash(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:8] + fmt.Sprintf("%08x", hashAny([]byte(s)))[:7]
}

// hashAny is PostgreSQL's hash_any() (Bob Jenkins' lookup3, as in
// src/common/hashfn.c) for little-endian platforms, which is what the
// pgEdge images run on.
func hashAny(k []byte) uint32 {
	a := 0x9e3779b9 + uint32(len(k)) + 3923095 // #nosec G115 -- same wraparound as C's uint32 length; inputs are short names
	b, c := a, a
	for len(k) >= 12 {
		a += binary.LittleEndian.Uint32(k[0:])
		b += binary.LittleEndian.Uint32(k[4:])
		c += binary.LittleEndian.Uint32(k[8:])
		a, b, c = hashMix(a, b, c)
		k = k[12:]
	}
	// Remaining 0-11 bytes; the lowest byte of c is reserved for the length.
	var tail [12]byte
	copy(tail[:], k)
	a += binary.LittleEndian.Uint32(tail[0:])
	b += binary.LittleEndian.Uint32(tail[4:])
	c += binary.LittleEndian.Uint32(tail[8:]) << 8
	return hashFinal(a, b, c)
}

func hashMix(a, b, c uint32) (uint32, uint32, uint32) {
	a -= c
	a ^= bits.RotateLeft32(c, 4)
	c += b
	b -= a
	b ^= bits.RotateLeft32(a, 6)
	a += c
	c -= b
	c ^= bits.RotateLeft32(b, 8)
	b += a
	a -= c
	a ^= bits.RotateLeft32(c, 16)
	c += b
	b -= a
	b ^= bits.RotateLeft32(a, 19)
	a += c
	c -= b
	c ^= bits.RotateLeft32(b, 4)
	b += a
	return a, b, c
}

func hashFinal(a, b, c uint32) uint32 {
	c ^= b
	c -= bits.RotateLeft32(b, 14)
	a ^= c
	a -= bits.RotateLeft32(c, 11)
	b ^= a
	b -= bits.RotateLeft32(a, 25)
	c ^= b
	c -= bits.RotateLeft32(b, 16)
	a ^= c
	a -= bits.RotateLeft32(c, 4)
	b ^= a
	b -= bits.RotateLeft32(a, 14)
	c ^= b
	c -= bits.RotateLeft32(b, 24)
	return c
}

// VerifySlotNames asks each provider's Spock for the slot name of every
// subscription and fails if spockSlotName disagrees. A wrong name makes the
// reconciler drop live slots and recreate healthy subscriptions, so stop
// before touching anything.
func VerifySlotNames(ctx context.Context, cfg *config.Config, conns map[string]*pgxpool.Pool) error {
	for _, src := range cfg.Nodes {
		for _, dst := range cfg.Nodes {
			if src.Name == dst.Name {
				continue
			}
			var spockName string
			err := conns[src.Name].QueryRow(ctx,
				"SELECT spock.spock_gen_slot_name($1, $2, $3)::text",
				cfg.DBName, src.Name, spockSubName(src.Name, dst.Name),
			).Scan(&spockName)
			if err != nil {
				return fmt.Errorf("get spock slot name for %s→%s: %w", src.Name, dst.Name, err)
			}
			if ours := spockSlotName(cfg.DBName, src.Name, dst.Name); ours != spockName {
				return fmt.Errorf("slot name mismatch for %s→%s: computed %q, spock uses %q", src.Name, dst.Name, ours, spockName)
			}
		}
	}
	return nil
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
