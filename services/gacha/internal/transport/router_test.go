package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	videoutils "github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeImageStore always returns a not-found error, simulating a store miss.
type fakeImageStore struct{}

func (f *fakeImageStore) Download(_ context.Context, _ string) (io.ReadCloser, *videoutils.VideoFile, error) {
	return nil, nil, errors.New("not found")
}

// fakeObjectStore satisfies the service.objectStore interface (Upload only).
type fakeObjectStore struct{}

func (f *fakeObjectStore) Upload(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return errors.New("not implemented")
}

// testRouter is a package-level singleton to avoid duplicate Prometheus
// metric registrations across sub-tests (metrics.NewCollector uses promauto
// which registers with the global registry and panics on duplicates).
var (
	testRouter     http.Handler
	testRouterOnce sync.Once
)

func getTestRouter(t *testing.T) http.Handler {
	t.Helper()
	testRouterOnce.Do(func() {
		log := logger.Default()
		mc := metrics.NewCollector("gacha-test")
		jwtCfg := authz.JWTConfig{
			Secret:         "test-secret",
			Issuer:         "test",
			AccessTokenTTL: time.Minute,
		}

		// Build a minimal sqlite DB for the wallet service.
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			panic("open sqlite: " + err.Error())
		}
		for _, s := range []string{
			`CREATE TABLE gacha_wallets (
				user_id TEXT PRIMARY KEY,
				balance INTEGER NOT NULL DEFAULT 0,
				starter_granted INTEGER NOT NULL DEFAULT 0,
				daily_streak INTEGER NOT NULL DEFAULT 0,
				last_daily_at DATETIME,
				created_at DATETIME,
				updated_at DATETIME
			)`,
			`CREATE TABLE gacha_ledger (
				id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
				user_id TEXT NOT NULL,
				delta INTEGER NOT NULL,
				reason TEXT NOT NULL,
				ref TEXT NOT NULL DEFAULT '',
				created_at DATETIME
			)`,
			`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
		} {
			if err := db.Exec(s).Error; err != nil {
				panic("DDL: " + err.Error())
			}
		}

		walletRepo := repo.NewWalletRepository(db)
		walletSvc := service.NewWalletService(walletRepo, 100, 50, 10, 100, true, log)
		walletH := handler.NewWalletHandler(walletSvc, log)
		internalH := handler.NewInternalHandler(walletSvc, log)

		// For admin handler, we only need the router/auth behaviour (requests
		// will fail at auth before hitting the DB).
		contentDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		contentRepo := repo.NewContentRepository(contentDB)
		bannerRepo := repo.NewBannerRepository(contentDB)
		contentSvc := service.NewContentService(contentRepo, bannerRepo)
		imageSvc := service.NewImageService(&fakeObjectStore{})
		adminH := handler.NewAdminHandler(contentSvc, imageSvc, log)
		imagesH := handler.NewImagesHandler(&fakeImageStore{}, log)

		pullRepo := repo.NewPullRepository(contentDB)
		pullSvc := service.NewPullService(pullRepo, bannerRepo, contentRepo, config.EconomyConfig{
			PullCostX1: 100, PullCostX10: 900, PityThreshold: 90,
			WeightN: 69, WeightR: 22, WeightSR: 8, WeightSSR: 1,
		}, service.NewSecureRand(), true, log)
		pullH := handler.NewPullHandler(pullSvc, log)

		testRouter = NewRouter(walletH, internalH, adminH, imagesH, pullH, jwtCfg, log, mc)
	})
	return testRouter
}

// TestRouter_ImagesRoute_NoAuth asserts that GET /api/gacha/images/cards/nope.png
// returns 404 (store miss) and NOT 401 — the images route is public.
func TestRouter_ImagesRoute_NoAuth(t *testing.T) {
	r := getTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gacha/images/cards/nope.png", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("images route: expected 404, got %d (route must be public; store miss → 404)", rr.Code)
	}
}

// TestRouter_Wallet_RequiresAuth asserts that GET /api/gacha/wallet without a
// JWT returns 401.
func TestRouter_Wallet_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gacha/wallet", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wallet route: expected 401 without token, got %d", rr.Code)
	}
}

// TestRouter_Banners_RequiresAuth asserts GET /api/gacha/banners → 401 without token.
func TestRouter_Banners_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/gacha/banners", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("banners route: expected 401 without token, got %d", rr.Code)
	}
}

// TestRouter_Pull_RequiresAuth asserts POST /api/gacha/banners/{id}/pull → 401 without token.
func TestRouter_Pull_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)
	req := httptest.NewRequest(http.MethodPost,
		"/api/gacha/banners/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/pull", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("pull route: expected 401 without token, got %d", rr.Code)
	}
}

// TestRouter_Collection_RequiresAuth asserts GET /api/gacha/collection → 401 without token.
func TestRouter_Collection_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/gacha/collection", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("collection route: expected 401 without token, got %d", rr.Code)
	}
}

// TestRouter_Admin_RequiresAuth asserts that GET /api/gacha/admin/cards without
// a JWT returns 401.
func TestRouter_Admin_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gacha/admin/cards", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("admin cards route: expected 401 without token, got %d", rr.Code)
	}
}

// TestRouter_AdminBulk_RequiresAuth asserts the bulk card routes exist and are
// auth-gated (401 without token — NOT 404/405, which would mean a routing bug).
func TestRouter_AdminBulk_RequiresAuth(t *testing.T) {
	r := getTestRouter(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPatch, "/api/gacha/admin/cards/bulk"},
		{http.MethodPost, "/api/gacha/admin/cards/bulk-delete"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401 without token, got %d", tc.method, tc.path, rr.Code)
		}
	}
}
