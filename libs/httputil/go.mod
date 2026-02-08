module github.com/ILITA-hub/animeenigma/libs/httputil

go 1.22

require (
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/go-chi/chi/v5 v5.0.12
	github.com/go-chi/render v1.0.3
)

replace (
	github.com/ILITA-hub/animeenigma/libs/errors => ../errors
	github.com/ILITA-hub/animeenigma/libs/logger => ../logger
)

require (
	github.com/ajg/form v1.5.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	go.opentelemetry.io/otel v1.24.0 // indirect
	go.opentelemetry.io/otel/trace v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240228224816-df926f6c8641 // indirect
	google.golang.org/grpc v1.62.0 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
)
