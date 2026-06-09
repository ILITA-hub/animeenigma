module github.com/ILITA-hub/animeenigma/services/gacha

go 1.24.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/prometheus/client_golang v1.23.2
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)
