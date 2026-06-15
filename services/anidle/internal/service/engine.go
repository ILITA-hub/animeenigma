package service

import "github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"

// Compare scores a guess against the secret, per spec §2.3. Pure function.
func Compare(secret, guess domain.PoolAnime) domain.GuessComparison {
	return domain.GuessComparison{
		Genres:   compareSet(taxonIDs(secret.Genres), taxonIDs(guess.Genres)),
		Studios:  compareSet(taxonIDs(secret.Studios), taxonIDs(guess.Studios)),
		Tags:     compareSet(taxonIDs(secret.Tags), taxonIDs(guess.Tags)),
		Year:     compareInt(secret.Year, guess.Year),
		Episodes: compareInt(secret.Episodes, guess.Episodes),
		Score:    compareFloat(secret.Score, guess.Score),
		Status:   compareEnum(secret.Status, guess.Status),
		Rating:   compareEnum(secret.Rating, guess.Rating),
	}
}

func taxonIDs(ts []domain.Taxon) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[t.ID] = struct{}{}
	}
	return m
}

func compareSet(secret, guess map[string]struct{}) domain.ColumnResult {
	if len(secret) == len(guess) {
		equal := true
		for id := range secret {
			if _, ok := guess[id]; !ok {
				equal = false
				break
			}
		}
		if equal {
			return domain.ColumnResult{Status: domain.MatchCorrect}
		}
	}
	for id := range guess {
		if _, ok := secret[id]; ok {
			return domain.ColumnResult{Status: domain.MatchPartial}
		}
	}
	return domain.ColumnResult{Status: domain.MatchWrong}
}

func compareInt(secret, guess int) domain.ColumnResult {
	switch {
	case secret == guess:
		return domain.ColumnResult{Status: domain.MatchCorrect}
	case secret > guess:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintHigher}
	default:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintLower}
	}
}

func compareFloat(secret, guess float64) domain.ColumnResult {
	switch {
	case secret == guess:
		return domain.ColumnResult{Status: domain.MatchCorrect}
	case secret > guess:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintHigher}
	default:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintLower}
	}
}

func compareEnum(secret, guess string) domain.ColumnResult {
	if secret == guess {
		return domain.ColumnResult{Status: domain.MatchCorrect}
	}
	return domain.ColumnResult{Status: domain.MatchWrong}
}
