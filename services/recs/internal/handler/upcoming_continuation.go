// Package handler — upcoming_continuation.go: detects whether an announced
// title is a CONTINUATION (a later entry of an existing series) vs a standalone
// / first entry. Continuations may only surface through the franchise signal
// (the user scored a prior entry) — never through the taste path — so the
// admission gate in upcoming.go must know which candidates are continuations
// (spec 2026-07-18 §1).
//
// Two independent detectors, OR'd by the caller:
//   - looksLikeSequel: pure name heuristic (EN + RU markers). Catches sequels
//     even when franchise data is unenriched (the Witch Watch case: both rows
//     have an empty franchise, so the structural check below can't fire).
//   - franchiseHasAiredSibling: structural — the candidate's franchise already
//     has a released/ongoing entry, so this announced title is a later entry.
//     Catches subtitle-named sequels the name heuristic misses, once franchises
//     are enriched.
package handler

import (
	"context"
	"regexp"
	"strings"
)

// sequelPatterns match names that denote a LATER entry in a series. All are
// case-insensitive; Unicode case folding covers the Cyrillic markers. The
// numeric markers deliberately exclude "1" (`(?:[2-9]|[1-9]\d+)` = 2..9 or
// 10+): a first season explicitly titled "Season 1" / "Part 1" / "1st Season"
// is NOT a continuation and must stay eligible for the taste path.
var sequelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(2nd|3rd|4th|5th|second|third|fourth|fifth|final)\s+season\b`),
	regexp.MustCompile(`(?i)\bseason\s+(?:[2-9]|[1-9]\d+)\b`),
	regexp.MustCompile(`(?i)\bpart\s+(?:[2-9]|[1-9]\d+|ii|iii|iv|v)\b`),
	regexp.MustCompile(`(?i)\b(?:[2-9]|[1-9]\d+)(?:st|nd|rd|th)\s+(?:season|part|cour)\b`),
	regexp.MustCompile(`(?i)\bcour\s+(?:[2-9]|[1-9]\d+)\b`),
	// Trailing standalone roman numeral (e.g. "Overlord IV"). Anchored to end
	// with a leading space so mid-name numerals don't trip it. "I" is excluded.
	regexp.MustCompile(`(?i)\s(ii|iii|iv|v|vi|vii|viii)$`),
	// Russian: "2 сезон" / "2-й сезон" / "сезон 2" / "часть 2" (never "1").
	regexp.MustCompile(`(?i)(?:[2-9]|[1-9]\d+)\s*[\p{Cyrillic}-]*\s*сезон`),
	regexp.MustCompile(`(?i)сезон\s*(?:[2-9]|[1-9]\d+)`),
	regexp.MustCompile(`(?i)часть\s+(?:[2-9]|[1-9]\d+)`),
}

// nameRow holds the two titles the sequel heuristic inspects.
type nameRow struct {
	Name   string
	NameRU string
}

// loadNames fetches name + name_ru for the given ids (for continuation
// detection). Missing ids simply map to the zero nameRow.
func (h *UpcomingHandler) loadNames(ctx context.Context, ids []string) (map[string]nameRow, error) {
	out := make(map[string]nameRow, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		ID     string
		Name   string
		NameRU string
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, name, name_ru").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = nameRow{Name: r.Name, NameRU: r.NameRU}
	}
	return out, nil
}

// looksLikeSequel reports whether either title reads as a continuation.
func looksLikeSequel(name, nameRU string) bool {
	for _, s := range []string{strings.TrimSpace(name), strings.TrimSpace(nameRU)} {
		if s == "" {
			continue
		}
		for _, re := range sequelPatterns {
			if re.MatchString(s) {
				return true
			}
		}
	}
	return false
}

// franchiseHasAiredSibling returns, for each candidate id, whether its franchise
// already has a released/ongoing entry (making this announced title a later
// entry). Candidates with an empty/unknown franchise map to false. Two batched
// queries regardless of pool size.
func (h *UpcomingHandler) franchiseHasAiredSibling(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// 1. Candidate → franchise (non-empty only).
	type candRow struct {
		ID        string
		Franchise string
	}
	var candRows []candRow
	if err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, franchise").
		Where("id IN ? AND franchise <> ''", ids).
		Scan(&candRows).Error; err != nil {
		return nil, err
	}
	if len(candRows) == 0 {
		return out, nil
	}

	franchiseSet := make(map[string]struct{}, len(candRows))
	for _, c := range candRows {
		franchiseSet[c.Franchise] = struct{}{}
	}
	franchises := make([]string, 0, len(franchiseSet))
	for f := range franchiseSet {
		franchises = append(franchises, f)
	}

	// 2. Which of those franchises have an aired (released/ongoing) entry.
	var airedFranchises []string
	if err := h.db.WithContext(ctx).
		Table("animes").
		Distinct("franchise").
		Where("franchise IN ? AND status IN ?", franchises, []string{"released", "ongoing"}).
		Pluck("franchise", &airedFranchises).Error; err != nil {
		return nil, err
	}
	airedSet := make(map[string]struct{}, len(airedFranchises))
	for _, f := range airedFranchises {
		airedSet[f] = struct{}{}
	}

	for _, c := range candRows {
		if _, ok := airedSet[c.Franchise]; ok {
			out[c.ID] = true
		}
	}
	return out, nil
}
