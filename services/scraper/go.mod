module github.com/ILITA-hub/animeenigma/services/scraper

go 1.24.0

require (
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/idmapping v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/streamprobe v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/ILITA-hub/animeenigma/libs/videoutils v0.0.0-00010101000000-000000000000
	github.com/PuerkitoBio/goquery v1.10.3
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c
	github.com/go-chi/chi/v5 v5.2.5
	github.com/hashicorp/go-retryablehttp v0.7.7
	github.com/prometheus/client_golang v1.23.2
	github.com/sebdah/goldie/v2 v2.5.5
	github.com/stretchr/testify v1.11.1
	golang.org/x/mod v0.30.0
	golang.org/x/net v0.47.0
	golang.org/x/time v0.14.0
)

require (
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-chi/render v1.0.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/minio-go/v7 v7.0.67 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/redis/go-redis/v9 v9.5.1 // indirect
	github.com/rs/xid v1.5.0 // indirect
	github.com/sergi/go-diff v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.63.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.38.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/crypto v0.44.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)

replace github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache

replace github.com/ILITA-hub/animeenigma/libs/streamprobe => ../../libs/streamprobe

replace github.com/ILITA-hub/animeenigma/libs/videoutils => ../../libs/videoutils

replace github.com/ILITA-hub/animeenigma/libs/idmapping => ../../libs/idmapping

// Phase 28 Plan 28-04: exclude the legacy top-level genproto module which
// ships a duplicate `googleapis/rpc/status` package now also exported from
// the modular `google.golang.org/genproto/googleapis/rpc` module. Without
// this exclude, go mod resolution after adding libs/idmapping reaches the
// torrent → genproto edge from libs/library via the workspace, triggering
// "ambiguous import" against grpc@v1.77.0. The legacy module is only
// touched transitively by anacrolix/torrent (used by services/library);
// excluding it forces all `googleapis/rpc/status` references to resolve
// against the modular sub-module.
exclude (
	google.golang.org/genproto v0.0.0-20221027153422-115e99e71e1c
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1
	google.golang.org/genproto v0.0.0-20240123012728-ef4313101c80
)
