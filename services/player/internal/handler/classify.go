package handler

// normalizeSource maps the historically free-text `source` field (and
// player_type) onto the four canonical Project-Board channels:
//
//	feedback_form — submitted via the in-site feedback button
//	telegram      — mirrored from Telegram by the maintenance bot
//	api           — created programmatically (agents, scripts, bin/feedback*,
//	                legacy owner-todo / repo-todo ledgers)
//	manual        — typed into the admin board via "+ New note"
//
// New code paths write a canonical value, which passes through unchanged;
// legacy items are derived from their raw signals.
func normalizeSource(rawSource, playerType string) string {
	switch rawSource {
	case "feedback_form", "telegram", "api", "manual":
		return rawSource
	}
	if playerType == "telegram" {
		return "telegram"
	}
	if rawSource == "" {
		return "feedback_form"
	}
	// Any other non-empty legacy source (owner-todo, repo-todo, …) was a
	// programmatic write.
	return "api"
}

// deriveKind returns the explicit kind when present and valid, otherwise infers
// it from the (already-normalized) source: user channels → feedback, internal
// channels → todo. There are no legacy "idea" items; ideas are only created
// going forward via "+ New note".
func deriveKind(rawKind, normalizedSource string) string {
	switch rawKind {
	case "feedback", "todo", "idea":
		return rawKind
	}
	switch normalizedSource {
	case "feedback_form", "telegram":
		return "feedback"
	default: // api, manual
		return "todo"
	}
}
