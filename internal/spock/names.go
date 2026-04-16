// internal/spock/names.go
package spock

import (
	"fmt"
	"strings"
)

// spockSlotName returns the canonical Spock replication slot name for a
// provider→subscriber pair. Dashes are replaced with underscores because
// PostgreSQL replication slot names cannot contain dashes.
func spockSlotName(dbName, providerName, subscriberName string) string {
	return strings.ReplaceAll(
		fmt.Sprintf("spk_%s_%s_sub_%s_%s", dbName, providerName, providerName, subscriberName),
		"-", "_",
	)
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
