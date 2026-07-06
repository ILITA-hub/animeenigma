package domain

// Tag is a curated fanfic tag with localized labels.
type Tag struct {
	Slug string `json:"slug"`
	RU   string `json:"ru"`
	EN   string `json:"en"`
}

// CuratedTags is the picker's suggestion list (users may also add free-text tags).
var CuratedTags = []Tag{
	{"fluff", "флафф", "fluff"},
	{"angst", "ангст", "angst"},
	{"slow-burn", "медленное развитие", "slow burn"},
	{"romance", "романтика", "romance"},
	{"comedy", "юмор", "comedy"},
	{"drama", "драма", "drama"},
	{"au", "AU", "AU"},
	{"hurt-comfort", "hurt/comfort", "hurt/comfort"},
	{"adventure", "приключения", "adventure"},
	{"friendship", "дружба", "friendship"},
}
