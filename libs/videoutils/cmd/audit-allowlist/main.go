// Command audit-allowlist prints the HLS proxy allow-list with provenance
// metadata (domain | owner | added | reason), one entry per line. It is the
// AST-aware backend for scripts/audit-hls-allowlist.sh — the script shells
// to `go run` of this package so the audit output is robust against
// reformatting or reordering of the struct slice in libs/videoutils/proxy.go.
//
// Usage:
//
//	go run ./libs/videoutils/cmd/audit-allowlist           # human-readable table
//	go run ./libs/videoutils/cmd/audit-allowlist -format=tsv   # TAB-separated, scriptable
//	go run ./libs/videoutils/cmd/audit-allowlist -format=csv   # CSV (RFC 4180 minimal)
//	go run ./libs/videoutils/cmd/audit-allowlist -legacy-only  # only entries owned by @legacy (backfill candidates)
//
// See docs/security/hls-proxy-allowlist.md for the quarterly review process.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

func main() {
	var (
		format     = flag.String("format", "table", "output format: table | tsv | csv")
		legacyOnly = flag.Bool("legacy-only", false, "show only entries owned by @legacy (backfill candidates)")
	)
	flag.Parse()

	entries := videoutils.HLSProxyAllowedDomainsWithProvenance
	if *legacyOnly {
		filtered := entries[:0:0]
		for _, e := range entries {
			if strings.EqualFold(e.Owner, "@legacy") {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	switch *format {
	case "table":
		printTable(entries)
	case "tsv":
		printDelimited(entries, '\t')
	case "csv":
		printCSV(entries)
	default:
		fmt.Fprintf(os.Stderr, "unknown format %q (want: table | tsv | csv)\n", *format)
		os.Exit(2)
	}

	// Trailing summary on stderr so it never pollutes scriptable output.
	fmt.Fprintf(os.Stderr, "\n# audit-hls-allowlist: %d entries", len(entries))
	if !*legacyOnly {
		legacy := 0
		for _, e := range videoutils.HLSProxyAllowedDomainsWithProvenance {
			if strings.EqualFold(e.Owner, "@legacy") {
				legacy++
			}
		}
		fmt.Fprintf(os.Stderr, " (%d owned by @legacy — backfill candidates)", legacy)
	}
	fmt.Fprintln(os.Stderr)
}

func printTable(entries []videoutils.AllowedDomain) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DOMAIN\tOWNER\tADDED\tREASON")
	fmt.Fprintln(tw, "------\t-----\t-----\t------")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Domain, e.Owner, e.Added, e.Reason)
	}
	tw.Flush()
}

func printDelimited(entries []videoutils.AllowedDomain, sep rune) {
	for _, e := range entries {
		fmt.Printf("%s%c%s%c%s%c%s\n", e.Domain, sep, e.Owner, sep, e.Added, sep, e.Reason)
	}
}

func printCSV(entries []videoutils.AllowedDomain) {
	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{"domain", "owner", "added", "reason"})
	for _, e := range entries {
		_ = w.Write([]string{e.Domain, e.Owner, e.Added, e.Reason})
	}
	w.Flush()
}
