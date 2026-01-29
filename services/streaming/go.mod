module github.com/ILITA-hub/animeenigma/services/streaming

go 1.22

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/videoutils v0.0.0
	github.com/go-chi/chi/v5 v5.0.12
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/videoutils => ../../libs/videoutils
)
