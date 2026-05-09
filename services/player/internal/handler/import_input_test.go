package handler

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMALUsername(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantErr     bool
		wantReason  string
		wantHostKey string
	}{
		{name: "bare username", input: "JohnDoe", want: "JohnDoe"},
		{name: "trims whitespace", input: "  JohnDoe  ", want: "JohnDoe"},
		{name: "username with underscore", input: "John_Doe123", want: "John_Doe123"},

		{name: "https profile url", input: "https://myanimelist.net/profile/JohnDoe", want: "JohnDoe"},
		{name: "https animelist url", input: "https://myanimelist.net/animelist/JohnDoe", want: "JohnDoe"},
		{name: "http profile url", input: "http://myanimelist.net/profile/JohnDoe", want: "JohnDoe"},
		{name: "url with trailing slash", input: "https://myanimelist.net/profile/JohnDoe/", want: "JohnDoe"},
		{name: "url with query string", input: "https://myanimelist.net/animelist/JohnDoe?status=1", want: "JohnDoe"},
		{name: "schemeless url", input: "myanimelist.net/profile/JohnDoe", want: "JohnDoe"},
		{name: "www prefix", input: "https://www.myanimelist.net/profile/JohnDoe", want: "JohnDoe"},
		{name: "bare host with path", input: "myanimelist.net/animelist/JohnDoe", want: "JohnDoe"},
		{name: "root level username on mal", input: "https://myanimelist.net/JohnDoe", want: "JohnDoe"},

		{name: "empty", input: "", wantErr: true, wantReason: "empty"},
		{name: "whitespace only", input: "   ", wantErr: true, wantReason: "empty"},
		{name: "username with slash", input: "John/Doe", wantErr: true, wantReason: "contains_separator"},
		{name: "username with @", input: "John@Doe", wantErr: true, wantReason: "contains_separator"},
		{name: "username with space", input: "John Doe", wantErr: true, wantReason: "contains_separator"},

		{name: "wrong host", input: "https://shikimori.one/JohnDoe", wantErr: true, wantReason: "url_wrong_host", wantHostKey: "shikimori.one"},
		{name: "wrong host bare", input: "https://example.com/profile/JohnDoe", wantErr: true, wantReason: "url_wrong_host", wantHostKey: "example.com"},

		{name: "mal root with no path", input: "https://myanimelist.net/", wantErr: true, wantReason: "url_no_username"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractMALUsername(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				appErr, ok := errors.IsAppError(err)
				require.True(t, ok, "expected AppError, got %T", err)
				assert.Equal(t, errors.CodeInvalidInput, appErr.Code)
				if tt.wantReason != "" {
					assert.Equal(t, tt.wantReason, appErr.Details["reason"])
				}
				if tt.wantHostKey != "" {
					assert.Equal(t, tt.wantHostKey, appErr.Details["host"])
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractShikimoriNickname(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantErr     bool
		wantReason  string
		wantHostKey string
	}{
		{name: "bare nickname", input: "JohnDoe", want: "JohnDoe"},
		{name: "cyrillic nickname", input: "Иван123", want: "Иван123"},
		{name: "trims whitespace", input: "  JohnDoe  ", want: "JohnDoe"},

		{name: "https shikimori.one url", input: "https://shikimori.one/JohnDoe", want: "JohnDoe"},
		{name: "https shikimori.me url", input: "https://shikimori.me/JohnDoe", want: "JohnDoe"},
		{name: "url with list path", input: "https://shikimori.one/JohnDoe/list/anime", want: "JohnDoe"},
		{name: "schemeless url", input: "shikimori.one/JohnDoe", want: "JohnDoe"},
		{name: "www prefix", input: "https://www.shikimori.one/JohnDoe", want: "JohnDoe"},
		{name: "trailing slash", input: "https://shikimori.one/JohnDoe/", want: "JohnDoe"},

		{name: "empty", input: "", wantErr: true, wantReason: "empty"},
		{name: "whitespace only", input: "   ", wantErr: true, wantReason: "empty"},
		{name: "nickname with slash", input: "John/Doe", wantErr: true, wantReason: "contains_separator"},
		{name: "nickname with space", input: "John Doe", wantErr: true, wantReason: "contains_separator"},

		{name: "wrong host", input: "https://myanimelist.net/profile/JohnDoe", wantErr: true, wantReason: "url_wrong_host", wantHostKey: "myanimelist.net"},
		{name: "shikimori root with no path", input: "https://shikimori.one/", wantErr: true, wantReason: "url_no_username"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractShikimoriNickname(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				appErr, ok := errors.IsAppError(err)
				require.True(t, ok, "expected AppError, got %T", err)
				assert.Equal(t, errors.CodeInvalidInput, appErr.Code)
				if tt.wantReason != "" {
					assert.Equal(t, tt.wantReason, appErr.Details["reason"])
				}
				if tt.wantHostKey != "" {
					assert.Equal(t, tt.wantHostKey, appErr.Details["host"])
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
