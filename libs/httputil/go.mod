module github.com/ILITA-hub/animeenigma/libs/httputil

go 1.22

require (
	github.com/go-chi/chi/v5 v5.0.12
	github.com/go-chi/render v1.0.3
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
)

replace (
	github.com/ILITA-hub/animeenigma/libs/errors => ../errors
	github.com/ILITA-hub/animeenigma/libs/logger => ../logger
)

require (
	github.com/ajg/form v1.5.1 // indirect
)
