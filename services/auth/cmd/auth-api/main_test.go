package main

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactWebhookErr(t *testing.T) {
	uerr := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/botSECRET123/deleteWebhook",
		Err: context.DeadlineExceeded,
	}
	got := redactWebhookErr(uerr)
	require.NotContains(t, got.Error(), "SECRET123")
	require.ErrorIs(t, got, context.DeadlineExceeded)

	plain := errors.New("boom")
	require.Equal(t, plain, redactWebhookErr(plain))
}
