package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

func TestProviderConstructorRegistryResolveCachesInstance(t *testing.T) {
	r := NewProviderConstructorRegistry()
	builds := 0
	if err := r.Register("fake", func(runtime ProviderRuntimeConfig) (domain.Provider, error) {
		builds++
		return &health.FakeProvider{NameVal: runtime.Name}, nil
	}); err != nil {
		t.Fatal(err)
	}

	first, ok, err := r.Resolve("fake", ProviderRuntimeConfig{Name: "fake-a"})
	if err != nil || !ok {
		t.Fatalf("first resolve: ok=%v err=%v", ok, err)
	}
	second, ok, err := r.Resolve("fake", ProviderRuntimeConfig{Name: "fake-a"})
	if err != nil || !ok {
		t.Fatalf("second resolve: ok=%v err=%v", ok, err)
	}
	if first != second || builds != 1 {
		t.Fatalf("registry did not cache instance: same=%v builds=%d", first == second, builds)
	}
}

func TestProviderConstructorRegistryUnknownKind(t *testing.T) {
	r := NewProviderConstructorRegistry()
	provider, ok, err := r.Resolve("missing", ProviderRuntimeConfig{Name: "missing"})
	if err != nil || ok || provider != nil {
		t.Fatalf("unknown resolve = (%v,%v,%v), want (nil,false,nil)", provider, ok, err)
	}
}

func TestProviderConstructorRegistryGenericKindBuildsEachRow(t *testing.T) {
	r := NewProviderConstructorRegistry()
	if err := r.Register("generic", func(runtime ProviderRuntimeConfig) (domain.Provider, error) {
		return &health.FakeProvider{NameVal: runtime.Name}, nil
	}); err != nil {
		t.Fatal(err)
	}
	first, ok, err := r.Resolve("generic", ProviderRuntimeConfig{Name: "mirror-a"})
	if err != nil || !ok {
		t.Fatalf("resolve mirror-a: ok=%v err=%v", ok, err)
	}
	second, ok, err := r.Resolve("generic", ProviderRuntimeConfig{Name: "mirror-b"})
	if err != nil || !ok {
		t.Fatalf("resolve mirror-b: ok=%v err=%v", ok, err)
	}
	if first.Name() != "mirror-a" || second.Name() != "mirror-b" || first == second {
		t.Fatalf("generic instances = %q/%q same=%v", first.Name(), second.Name(), first == second)
	}
}
