package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRunFailoverNamed_EmitsParserMetrics(t *testing.T) {
	// success path → status="success"
	before := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_ok", "find_id", "success"))
	_, name, err := runFailoverNamed(
		context.Background(), nil,
		[]domain.Provider{&fakeProvider{nameVal: "p3_ok"}}, nil, 0, "find_id",
		func(c context.Context, p domain.Provider) (string, error) { return "X", nil },
	)
	if err != nil || name != "p3_ok" {
		t.Fatalf("runFailoverNamed = (%q, %v), want (p3_ok, nil)", name, err)
	}
	after := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_ok", "find_id", "success"))
	if after != before+1 {
		t.Errorf("parser_requests_total{p3_ok,find_id,success} = %v, want %v", after, before+1)
	}

	// error path → status="error", and failover still advances
	eBefore := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_err", "get_stream", "error"))
	_, _, _ = runFailoverNamed(
		context.Background(), nil,
		[]domain.Provider{&fakeProvider{nameVal: "p3_err"}}, nil, 0, "get_stream",
		func(c context.Context, p domain.Provider) (string, error) { return "", errors.New("boom") },
	)
	eAfter := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_err", "get_stream", "error"))
	if eAfter != eBefore+1 {
		t.Errorf("parser_requests_total{p3_err,get_stream,error} = %v, want %v", eAfter, eBefore+1)
	}
}
