package service

import (
	"fmt"
	"sync"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// ProviderRuntimeConfig is the DB-owned input available to a constructor. A
// genuinely generic engine kind can use Name/BaseURL to instantiate many rows;
// native constructors may ignore Name and remain bound by DB validation.
type ProviderRuntimeConfig struct {
	Name    string
	BaseURL string
}

// ProviderConstructor builds one provider implementation for an engine kind.
// Constructors are registered once at process boot; DB roster rows select them
// through engine_kind. Instances are cached so roster refreshes only re-gate
// providers and never rebuild clients while requests are in flight.
type ProviderConstructor func(ProviderRuntimeConfig) (domain.Provider, error)

// ProviderConstructorRegistry is the sole executable-provider registry. The DB
// controls which registered constructors exist at runtime; code controls only
// how a supported engine kind is built.
type ProviderConstructorRegistry struct {
	mu           sync.Mutex
	constructors map[string]ProviderConstructor
	instances    map[string]providerInstance
}

type providerInstance struct {
	kind     string
	provider domain.Provider
}

func NewProviderConstructorRegistry() *ProviderConstructorRegistry {
	return &ProviderConstructorRegistry{
		constructors: make(map[string]ProviderConstructor),
		instances:    make(map[string]providerInstance),
	}
}

// Register adds an engine-kind constructor. Duplicate kinds are rejected so
// implementation ownership cannot silently depend on initialization order.
func (r *ProviderConstructorRegistry) Register(kind string, constructor ProviderConstructor) error {
	if kind == "" || constructor == nil {
		return fmt.Errorf("provider constructor: kind and constructor are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.constructors[kind]; exists {
		return fmt.Errorf("provider constructor %q already registered", kind)
	}
	r.constructors[kind] = constructor
	return nil
}

// Resolve returns the cached implementation for one DB row, constructing it
// once on first use. ok=false means the row referred to an unsupported kind.
func (r *ProviderConstructorRegistry) Resolve(kind string, runtime ProviderRuntimeConfig) (provider domain.Provider, ok bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if instance, exists := r.instances[runtime.Name]; exists && instance.kind == kind {
		return instance.provider, true, nil
	}
	constructor, ok := r.constructors[kind]
	if !ok {
		return nil, false, nil
	}
	provider, err = constructor(runtime)
	if err != nil {
		return nil, true, err
	}
	if provider == nil {
		return nil, true, fmt.Errorf("provider constructor %q returned nil", kind)
	}
	r.instances[runtime.Name] = providerInstance{kind: kind, provider: provider}
	return provider, true, nil
}
