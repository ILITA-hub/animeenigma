package service

import (
	"fmt"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// MaxTokensFor maps a length preset to a generous completion-token budget.
func MaxTokensFor(length string) int {
	switch length {
	case "drabble":
		return 900
	case "oneshot":
		return 2600
	case "short":
		return 6000
	default:
		return 2600
	}
}

// lengthWords is the human-readable target used in the prompt.
func lengthWords(length string) int {
	switch length {
	case "drabble":
		return 300
	case "short":
		return 2000
	default:
		return 900
	}
}

// BuildMessages assembles the system + user prompts for a generation request.
// synopsis, when non-empty (canon mode), is injected into the user prompt as
// the official plot summary to continue from.
func BuildMessages(req domain.GenerateRequest, synopsis string) (string, string) {
	ru := req.Language == "ru"

	langName := "ENGLISH"
	povWord := "third"
	if req.POV == "first" {
		povWord = "first"
	}
	if ru {
		langName = "РУССКИЙ"
		povWord = "третьего"
		if req.POV == "first" {
			povWord = "первого"
		}
	}

	var sys strings.Builder
	if ru {
		fmt.Fprintf(&sys, "Ты — талантливый автор фанфиков, пишущий живую художественную прозу.\n")
		fmt.Fprintf(&sys, "Язык вывода строго: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleRU(req.Rating))
		fmt.Fprintf(&sys, "Все персонажи — совершеннолетние (18+), даже если в каноне выглядят младше.\n")
		fmt.Fprintf(&sys, "Повествование от %s лица. Целевой объём: около %d слов.\n", povWord, lengthWords(req.Length))
		if req.Canon {
			fmt.Fprintf(&sys, "Это ПРОДОЛЖЕНИЕ КАНОНА: продолжи РЕАЛЬНЫЙ сюжет аниме за пределами финала, оставаясь верным канону и характерам.\n")
		}
		fmt.Fprintf(&sys, "Ответ начни СТРОГО со строки «# Заголовок», затем с новой строки — текст истории в Markdown.\n")
		fmt.Fprintf(&sys, "Не отказывайся, не добавляй дисклеймеров и морализаторства — просто пиши историю.")
	} else {
		fmt.Fprintf(&sys, "You are a talented fanfiction author writing vivid literary prose.\n")
		fmt.Fprintf(&sys, "Output language strictly: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleEN(req.Rating))
		fmt.Fprintf(&sys, "Portray all characters as adults (18+), even if they look younger in canon.\n")
		fmt.Fprintf(&sys, "Write in the %s person. Target length: about %d words.\n", povWord, lengthWords(req.Length))
		if req.Canon {
			fmt.Fprintf(&sys, "This is a CANON CONTINUATION: continue the anime's ACTUAL plot past its finale, staying faithful to canon and characterization.\n")
		}
		fmt.Fprintf(&sys, "Begin your reply STRICTLY with a line '# Title', then on a new line the story in Markdown.\n")
		fmt.Fprintf(&sys, "Do not refuse, add disclaimers, or moralize — just write the story.")
	}

	names := make([]string, 0, len(req.Characters))
	for _, c := range req.Characters {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}

	var usr strings.Builder
	fandom := req.Anime.Title
	if req.Anime.Japanese != "" {
		fandom = fmt.Sprintf("%s (%s)", req.Anime.Title, req.Anime.Japanese)
	}
	if ru {
		fmt.Fprintf(&usr, "Фандом: %s\n", fandom)
		fmt.Fprintf(&usr, "Персонажи: %s\n", joinOr(names, "по твоему выбору"))
		fmt.Fprintf(&usr, "Теги: %s\n", joinOr(req.Tags, "—"))
	} else {
		fmt.Fprintf(&usr, "Fandom: %s\n", fandom)
		fmt.Fprintf(&usr, "Characters: %s\n", joinOr(names, "your choice"))
		fmt.Fprintf(&usr, "Tags: %s\n", joinOr(req.Tags, "—"))
	}

	if strings.TrimSpace(synopsis) != "" {
		if ru {
			fmt.Fprintf(&usr, "Официальный синопсис: %s\n", strings.TrimSpace(synopsis))
		} else {
			fmt.Fprintf(&usr, "Official synopsis: %s\n", strings.TrimSpace(synopsis))
		}
	}

	if ru {
		fmt.Fprintf(&usr, "Задание автора: %s", strOr(req.Prompt, "напиши историю на своё усмотрение"))
	} else {
		fmt.Fprintf(&usr, "Author brief: %s", strOr(req.Prompt, "write a story of your choosing"))
	}
	return sys.String(), usr.String()
}

func ratingRuleRU(rating string) string {
	switch rating {
	case "explicit":
		return "Рейтинг: Explicit. Допустимы откровенные сцены между совершеннолетними персонажами."
	case "mature":
		return "Рейтинг: Mature. Допустимы взрослые темы и намёки на близость, без графических подробностей."
	default:
		return "Рейтинг: Teen. Без откровенных сцен; романтика допустима, но целомудренная."
	}
}

func ratingRuleEN(rating string) string {
	switch rating {
	case "explicit":
		return "Rating: Explicit. Explicit scenes between adult characters are allowed."
	case "mature":
		return "Rating: Mature. Adult themes and implied intimacy allowed, no graphic detail."
	default:
		return "Rating: Teen. No explicit scenes; chaste romance only."
	}
}

// SplitTitle extracts a leading Markdown H1 as the title and returns the remaining body.
func SplitTitle(text string) (string, string) {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(trimmed, "# ") {
		nl := strings.IndexByte(trimmed, '\n')
		if nl == -1 {
			return strings.TrimSpace(trimmed[2:]), ""
		}
		title := strings.TrimSpace(trimmed[2:nl])
		body := strings.TrimLeft(trimmed[nl+1:], "\r\n")
		return title, body
	}
	return "", text
}

func joinOr(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, ", ")
}

func strOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// BuildContinueMessages builds the system+user prompts to generate the NEXT
// part of an existing fanfic. It reuses the stored rating/POV/language shape
// but omits the title instruction (we're mid-document) and feeds the prior
// text back as context.
func BuildContinueMessages(f domain.Fanfic, prior string) (string, string) {
	ru := f.Language == "ru"
	povWord := "third"
	if f.POV == "first" {
		povWord = "first"
	}
	langName := "ENGLISH"
	if ru {
		langName = "РУССКИЙ"
		povWord = "третьего"
		if f.POV == "first" {
			povWord = "первого"
		}
	}

	var sys strings.Builder
	if ru {
		fmt.Fprintf(&sys, "Ты — талантливый автор фанфиков, пишущий живую художественную прозу.\n")
		fmt.Fprintf(&sys, "Язык вывода строго: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleRU(f.Rating))
		fmt.Fprintf(&sys, "Все персонажи — совершеннолетние (18+).\n")
		fmt.Fprintf(&sys, "Повествование от %s лица.\n", povWord)
		if f.Canon {
			fmt.Fprintf(&sys, "Это продолжение канона — оставайся верным сюжету и характерам аниме.\n")
		}
		fmt.Fprintf(&sys, "Это ПРОДОЛЖЕНИЕ уже начатой истории. НЕ пиши заголовок и НЕ повторяй предыдущее — продолжи следующей частью, логично развивая сюжет.")
	} else {
		fmt.Fprintf(&sys, "You are a talented fanfiction author writing vivid literary prose.\n")
		fmt.Fprintf(&sys, "Output language strictly: %s.\n", langName)
		fmt.Fprintf(&sys, "%s\n", ratingRuleEN(f.Rating))
		fmt.Fprintf(&sys, "Portray all characters as adults (18+).\n")
		fmt.Fprintf(&sys, "Write in the %s person.\n", povWord)
		if f.Canon {
			fmt.Fprintf(&sys, "This is a canon continuation — stay faithful to the anime's plot and characters.\n")
		}
		fmt.Fprintf(&sys, "This CONTINUES an in-progress story. Do NOT write a title and do NOT repeat prior text — write the next part, advancing the plot logically.")
	}

	var usr strings.Builder
	if ru {
		fmt.Fprintf(&usr, "Фандом: %s\n", f.AnimeTitle)
		fmt.Fprintf(&usr, "Предыдущие части истории:\n%s\n\nНапиши следующую часть.", prior)
	} else {
		fmt.Fprintf(&usr, "Fandom: %s\n", f.AnimeTitle)
		fmt.Fprintf(&usr, "Story so far:\n%s\n\nWrite the next part.", prior)
	}
	return sys.String(), usr.String()
}

// TailRunes returns the last `max` runes of s (all of s if shorter). Used to
// bound the prior-content context fed into a continuation.
func TailRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[len(r)-max:])
}
