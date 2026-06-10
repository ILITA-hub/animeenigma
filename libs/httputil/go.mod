module github.com/ILITA-hub/animeenigma/libs/httputil

go 1.25.0

require (
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/go-chi/render v1.0.3
)

replace (
	github.com/ILITA-hub/animeenigma/libs/errors => ../errors
	github.com/ILITA-hub/animeenigma/libs/logger => ../logger
)

require (
	github.com/ajg/form v1.5.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)
