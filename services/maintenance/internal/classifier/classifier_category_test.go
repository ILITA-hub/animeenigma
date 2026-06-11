package classifier

import "testing"

func TestGuessCategory(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			// 2026-06-11T00-09-19_tNeymik_telegram: "lag" matched inside
			// "feature flags" and the explicit "Feature request" header was ignored.
			name: "explicit feature request with feature flags mention",
			text: "Feature request\n\nПодумать как сделать (и есть ли смысл) чтобы набор маленьких фиксов (спрятать кнопку/добавить кнопку, как Oronemu делал последние несколько задач)\nСначала работали ТОЛЬКО для него, а затем предлагалось масштабировать изменение на всех юзеров\n\nМб использовать механизм feature flags",
			want: "feature",
		},
		{name: "feature flags alone is not a bug", text: "use feature flags for rollout", want: "issue"},
		{name: "download is not down", text: "please download the new schedule", want: "issue"},
		{name: "russian breakage", text: "плеер не работает на втором эпизоде", want: "bug"},
		{name: "english breakage", text: "the video player is broken", want: "bug"},
		{name: "lagging still detected", text: "video is lagging on 1080p", want: "bug"},
		{name: "plain lag detected", text: "there is lag in the player", want: "bug"},
		{name: "russian feature phrasing", text: "Предложение: хотелось бы тёмную тему в плеере", want: "feature"},
		{name: "neutral message", text: "какое аниме посмотреть на выходных?", want: "issue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GuessCategory(tc.text); got != tc.want {
				t.Errorf("GuessCategory(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
