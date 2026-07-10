package domain

// Storage placement vocabulary shared by the library service's own code and the
// internal storage service (services/storage/). These string literals are a
// cross-service CONTRACT: they must stay byte-identical to
// services/storage/internal/domain/storage.go. They are re-declared here (rather
// than imported) because the storage service is a separate Go module consumed
// across a process boundary — the library never imports its internal packages.

// Content classes — the placement-policy keys the storage service resolves to a
// concrete backend. The library encoder picks ClassLibraryAuto for autocache
// (Planner-driven) downloads and ClassLibraryManual for every other ingest
// (admin upload / manual folder ingest / batchingest); only ClassLibraryManual
// honors a per-job storage override.
const (
	ClassLibraryAuto   = "library-auto"
	ClassLibraryManual = "library-manual"
)

// Backend ids — the two object stores the storage service brokers. Used as the
// storage-preference argument on episode lookups and as the fixed backend the
// legacy-admin migrator relocates within (all legacy admin content is local).
const (
	BackendMinio = "minio"
	BackendS3    = "s3"
)
