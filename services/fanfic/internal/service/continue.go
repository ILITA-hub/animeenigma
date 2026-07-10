package service

import (
	"context"
	"fmt"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// Continue generates the next part of an existing, complete fanfic and appends
// it to that fanfic's content, sectioned by a divider + «Часть N» heading. It
// reuses every stored parameter (length/POV/rating/language/canon); the prior
// content is fed back as context, bounded to contextRunes.
func (g *Generator) Continue(ctx context.Context, userID, id string, emit Emit) error {
	f, err := g.store.Get(ctx, userID, id)
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}
	if f.Status != domain.StatusComplete {
		e := liberrors.New(liberrors.CodeConflict, "fanfic is not complete")
		g.safeEmit(emit, "error", map[string]any{"message": e.Error()})
		return e
	}

	release, err := g.quota.Acquire(ctx, userID)
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}
	defer release()

	part := f.PartCount + 1
	heading := headingWord(f.Language)
	prefix := fmt.Sprintf("\n\n---\n\n## %s %d\n\n", heading, part)

	g.safeEmit(emit, "meta", map[string]any{"id": f.ID, "model": g.model, "part": part})
	// Emit the divider+heading first so the live reader matches the stored form.
	g.safeEmit(emit, "delta", map[string]any{"text": prefix})

	prior := TailRunes(f.Content, g.contextRunes)
	system, user := BuildContinueMessages(*f, prior)

	text, usage, err := g.groq.Stream(ctx, system, user, MaxTokensFor(f.Length), 0.9, func(delta string) {
		g.safeEmit(emit, "delta", map[string]any{"text": delta})
	})
	if err != nil {
		g.safeEmit(emit, "error", map[string]any{"message": err.Error()})
		return err
	}

	// Strip any stray leading title the model emitted; keep the body.
	_, body := SplitTitle(text)
	if body == "" {
		body = text
	}
	appended := prefix + body
	if err := g.store.AppendPart(ctx, userID, id, appended, usage, part); err != nil {
		if g.log != nil {
			g.log.Errorw("failed to append fanfic part", "id", id, "error", err)
		}
	}
	if g.log != nil {
		g.log.Infow("fanfic continued", "user_id", userID, "fanfic_id", id, "action", "continue",
			"canon", f.Canon, "part", part, "token_usage", usage, "status", "complete")
	}
	g.safeEmit(emit, "done", map[string]any{"id": f.ID, "part": part, "token_usage": usage})
	return nil
}

// headingWord returns the localized «part» heading word.
func headingWord(language string) string {
	if language == "ru" {
		return "Часть"
	}
	return "Part"
}
