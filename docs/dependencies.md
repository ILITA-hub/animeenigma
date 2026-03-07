# Dependency Management

## Vulnerability Scanning

### Go Backend
```bash
# Install govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest

# Scan all services (from project root)
for svc in services/*/; do
  echo "=== $svc ==="
  cd "$svc" && govulncheck ./... && cd ../..
done

# Scan shared libraries
for lib in libs/*/; do
  echo "=== $lib ==="
  cd "$lib" && govulncheck ./... && cd ../..
done
```

### Frontend
```bash
cd frontend/web
bun audit
```

## Update Procedures

### Go Dependencies
```bash
# Update a specific dependency across the workspace
cd libs/database
go get github.com/jackc/pgx/v5@latest
cd ../..
go work sync

# Update all dependencies in a module
cd services/catalog
go get -u ./...
go mod tidy
```

### Go Runtime
1. Update `FROM golang:X.YZ-alpine` in all `services/*/Dockerfile`
2. Update `go X.YZ` in `go.work`
3. Optionally update `go X.YZ` in each `go.mod` (sets minimum version)
4. Run `go work sync`

### Frontend Dependencies
```bash
cd frontend/web
bun update           # Update all to latest compatible
bun update <package> # Update specific package
```

## Key Dependencies

### Go Shared Libraries (`libs/`)
| Module | Key Dependencies | Purpose |
|--------|-----------------|---------|
| `database` | `gorm.io/gorm`, `jackc/pgx/v5` | PostgreSQL ORM + driver |
| `cache` | `redis/go-redis/v9` | Redis caching |
| `authz` | `golang-jwt/jwt/v5` | JWT token management |
| `httputil` | `go-chi/chi/v5`, `go-chi/render` | HTTP middleware & responses |
| `videoutils` | `minio/minio-go/v7` | Video proxy & MinIO client |
| `metrics` | `prometheus/client_golang` | Prometheus metrics |
| `logger` | `go.uber.org/zap` | Structured logging |
| `idmapping` | (stdlib only) | ARM anime ID mapping |

### Frontend (`frontend/web/`)
| Package | Purpose |
|---------|---------|
| `vue` | UI framework |
| `vite` | Build tool |
| `video.js` / `hls.js` | Video playback |
| `axios` | HTTP client |
| `pinia` | State management |
| `vue-router` | Client-side routing |

## Known Issues
- `libs/videoutils/go.mod` lists `stretchr/testify` as indirect — used for tests
- pgx must be kept at v5.5.4+ to avoid SQL injection CVE (GO-2024-2606)
