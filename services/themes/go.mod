module github.com/ILITA-hub/animeenigma/services/themes

go 1.24.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/go-chi/chi/v5 v5.0.12
	gorm.io/gorm v1.30.0
)

require (
	github.com/ajg/form v1.5.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-chi/render v1.0.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.4 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.44.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gorm.io/driver/postgres v1.5.7 // indirect
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
)
