package animeparser

import (
	"fmt"
	"time"
)

// AnimeSeason represents an anime broadcast season
type AnimeSeason string

const (
	SeasonWinter AnimeSeason = "winter" // January - March
	SeasonSpring AnimeSeason = "spring" // April - June
	SeasonSummer AnimeSeason = "summer" // July - September
	SeasonFall   AnimeSeason = "fall"   // October - December
)

// SeasonPeriod represents a specific anime season in a year
type SeasonPeriod struct {
	Year   int
	Season AnimeSeason
}

func (sp SeasonPeriod) String() string {
	return fmt.Sprintf("%s %d", sp.Season, sp.Year)
}

// StartDate returns the start date of this season
func (sp SeasonPeriod) StartDate() time.Time {
	month := sp.startMonth()
	return time.Date(sp.Year, month, 1, 0, 0, 0, 0, time.UTC)
}

// EndDate returns the end date of this season
func (sp SeasonPeriod) EndDate() time.Time {
	month := sp.endMonth()
	// Get the last day of the month
	return time.Date(sp.Year, month+1, 0, 23, 59, 59, 0, time.UTC)
}

func (sp SeasonPeriod) startMonth() time.Month {
	switch sp.Season {
	case SeasonWinter:
		return time.January
	case SeasonSpring:
		return time.April
	case SeasonSummer:
		return time.July
	case SeasonFall:
		return time.October
	default:
		return time.January
	}
}

func (sp SeasonPeriod) endMonth() time.Month {
	switch sp.Season {
	case SeasonWinter:
		return time.March
	case SeasonSpring:
		return time.June
	case SeasonSummer:
		return time.September
	case SeasonFall:
		return time.December
	default:
		return time.March
	}
}

// Next returns the next season
func (sp SeasonPeriod) Next() SeasonPeriod {
	switch sp.Season {
	case SeasonWinter:
		return SeasonPeriod{Year: sp.Year, Season: SeasonSpring}
	case SeasonSpring:
		return SeasonPeriod{Year: sp.Year, Season: SeasonSummer}
	case SeasonSummer:
		return SeasonPeriod{Year: sp.Year, Season: SeasonFall}
	case SeasonFall:
		return SeasonPeriod{Year: sp.Year + 1, Season: SeasonWinter}
	default:
		return sp
	}
}

// Previous returns the previous season
func (sp SeasonPeriod) Previous() SeasonPeriod {
	switch sp.Season {
	case SeasonWinter:
		return SeasonPeriod{Year: sp.Year - 1, Season: SeasonFall}
	case SeasonSpring:
		return SeasonPeriod{Year: sp.Year, Season: SeasonWinter}
	case SeasonSummer:
		return SeasonPeriod{Year: sp.Year, Season: SeasonSpring}
	case SeasonFall:
		return SeasonPeriod{Year: sp.Year, Season: SeasonSummer}
	default:
		return sp
	}
}

// DetectSeasonFromDate determines the anime season for a given date
func DetectSeasonFromDate(t time.Time) SeasonPeriod {
	year := t.Year()
	month := t.Month()

	var season AnimeSeason
	switch {
	case month >= time.January && month <= time.March:
		season = SeasonWinter
	case month >= time.April && month <= time.June:
		season = SeasonSpring
	case month >= time.July && month <= time.September:
		season = SeasonSummer
	default:
		season = SeasonFall
	}

	return SeasonPeriod{Year: year, Season: season}
}

// CurrentSeason returns the current anime season
func CurrentSeason() SeasonPeriod {
	return DetectSeasonFromDate(time.Now())
}

// UpcomingSeason returns the next anime season
func UpcomingSeason() SeasonPeriod {
	return CurrentSeason().Next()
}

// ParseSeason parses a season string like "winter 2024" or "fall2023"
func ParseSeason(s string) (SeasonPeriod, error) {
	var season AnimeSeason
	var year int

	// Try various formats
	formats := []string{
		"%s %d",   // "winter 2024"
		"%s%d",    // "winter2024"
		"%d %s",   // "2024 winter"
	}

	var seasonStr string
	for _, format := range formats {
		n, _ := fmt.Sscanf(s, format, &seasonStr, &year)
		if n == 2 {
			break
		}
		n, _ = fmt.Sscanf(s, format, &year, &seasonStr)
		if n == 2 {
			break
		}
	}

	switch seasonStr {
	case "winter", "Winter", "WINTER":
		season = SeasonWinter
	case "spring", "Spring", "SPRING":
		season = SeasonSpring
	case "summer", "Summer", "SUMMER":
		season = SeasonSummer
	case "fall", "Fall", "FALL", "autumn", "Autumn", "AUTUMN":
		season = SeasonFall
	default:
		return SeasonPeriod{}, fmt.Errorf("invalid season: %s", seasonStr)
	}

	if year < 1900 || year > 2100 {
		return SeasonPeriod{}, fmt.Errorf("invalid year: %d", year)
	}

	return SeasonPeriod{Year: year, Season: season}, nil
}

// GetSeasonRange returns all seasons in a range (inclusive)
func GetSeasonRange(start, end SeasonPeriod) []SeasonPeriod {
	var seasons []SeasonPeriod
	current := start

	for {
		seasons = append(seasons, current)
		if current.Year == end.Year && current.Season == end.Season {
			break
		}
		if current.Year > end.Year || (current.Year == end.Year && seasonOrder(current.Season) > seasonOrder(end.Season)) {
			break
		}
		current = current.Next()
	}

	return seasons
}

func seasonOrder(s AnimeSeason) int {
	switch s {
	case SeasonWinter:
		return 0
	case SeasonSpring:
		return 1
	case SeasonSummer:
		return 2
	case SeasonFall:
		return 3
	default:
		return 0
	}
}
