package sources

import (
	"fmt"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"slices"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Definition struct {
	Code               string
	CredentialSelector string
	Load               func(defaultTimeout time.Duration) (ConfiguredSource, bool, error)
	DecodeCredentials  func(fields map[string]string) (capability.Access, error)
}

type BuildOptions struct {
	SkipTLSVerify bool
}

// ConfiguredSource captures server settings only; credentials arrive per request.
type ConfiguredSource struct {
	Fingerprint string
	Code        string
	InstanceIDs []string
	Build       func(BuildOptions) (registry.Provider, error)
}

type Catalog struct {
	definitions []Definition
	byCode      map[string]Definition
	bySelector  map[string]Definition
}

func Default() (*Catalog, error) {
	return New(maxpatrolDefinition(), ptnadDefinition(), wazuhDefinition())
}

func New(definitions ...Definition) (*Catalog, error) {
	catalog := &Catalog{
		definitions: slices.Clone(definitions),
		byCode:      make(map[string]Definition, len(definitions)),
		bySelector:  make(map[string]Definition, len(definitions)),
	}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.Code) == "" || definition.Code != strings.TrimSpace(definition.Code) {
			return nil, fmt.Errorf("source code must be nonempty and have no surrounding whitespace")
		}
		if strings.TrimSpace(definition.CredentialSelector) == "" || definition.Load == nil || definition.DecodeCredentials == nil {
			return nil, fmt.Errorf("source %s requires a credential selector, loader and credentials decoder", definition.Code)
		}
		if _, exists := catalog.byCode[definition.Code]; exists {
			return nil, fmt.Errorf("source %s is registered twice", definition.Code)
		}
		if _, exists := catalog.bySelector[definition.CredentialSelector]; exists {
			return nil, fmt.Errorf("source credential selector is registered twice")
		}
		catalog.byCode[definition.Code] = definition
		catalog.bySelector[definition.CredentialSelector] = definition
	}
	return catalog, nil
}

func (catalog *Catalog) Load(defaultTimeout time.Duration) (map[string]ConfiguredSource, error) {
	configured := make(map[string]ConfiguredSource, len(catalog.definitions))
	for _, definition := range catalog.definitions {
		source, enabled, err := definition.Load(defaultTimeout)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		if source.Code != definition.Code || source.Build == nil {
			return nil, fmt.Errorf("source %s returned invalid backend configuration", definition.Code)
		}
		configured[definition.Code] = source
	}
	return configured, nil
}

func (catalog *Catalog) Build(configured map[string]ConfiguredSource, options BuildOptions) (*registry.Registry, error) {
	codes := make([]string, 0, len(configured))
	for code := range configured {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	providers := make([]registry.Provider, 0, len(codes))
	for _, code := range codes {
		definition, exists := catalog.byCode[code]
		if !exists {
			return nil, fmt.Errorf("unknown source %s", code)
		}
		source := configured[code]
		if source.Code != code || source.Build == nil {
			return nil, fmt.Errorf("source %s has invalid backend configuration", code)
		}
		provider, err := source.Build(options)
		if err != nil {
			return nil, fmt.Errorf("configure %s: %w", code, err)
		}
		if provider.Source.Code != code || provider.CredentialSecret != definition.CredentialSelector {
			return nil, fmt.Errorf("source %s returned a provider with mismatched identity or credential selector", code)
		}
		providers = append(providers, provider)
	}
	return registry.New(providers...)
}

// ResolveCredentials gives each decoder only its own source object from the snapshot.
func (catalog *Catalog) ResolveCredentials(configuration map[string]map[string]string, selectors ...string) (map[string]capability.Access, error) {
	result := make(map[string]capability.Access, len(selectors))
	for _, selector := range selectors {
		definition, exists := catalog.bySelector[selector]
		if !exists {
			return nil, fmt.Errorf("unknown configuration credential selector")
		}
		cookie, err := definition.DecodeCredentials(configuration[definition.Code])
		if err != nil {
			return nil, err
		}
		result[selector] = cookie
	}
	return result, nil
}
