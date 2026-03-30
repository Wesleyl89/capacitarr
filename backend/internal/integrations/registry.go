package integrations

import (
	"fmt"
	"log/slog"
	"sync"
)

// IntegrationRegistry stores registered integration clients keyed by integration ID.
// Consumers ask for clients by capability (e.g. "all WatchDataProviders") instead of
// checking named fields. Adding a new integration type requires zero changes to consumers.
type IntegrationRegistry struct {
	mu sync.RWMutex

	connectors              map[uint]Connectable
	mediaSources            map[uint]MediaSource
	diskReporters           map[uint]DiskReporter
	deleters                map[uint]MediaDeleter
	watchProviders          map[uint]WatchDataProvider
	requestProviders        map[uint]RequestProvider
	watchlistProviders      map[uint]WatchlistProvider
	ruleValueFetchers       map[uint]RuleValueFetcher
	collectionResolvers     map[uint]CollectionResolver
	collectionDataProviders map[uint]CollectionDataProvider
	labelDataProviders      map[uint]LabelDataProvider
	labelManagers           map[uint]LabelManager
	posterManagers          map[uint]PosterManager
}

// NewIntegrationRegistry creates an empty registry.
func NewIntegrationRegistry() *IntegrationRegistry {
	return &IntegrationRegistry{
		connectors:              make(map[uint]Connectable),
		mediaSources:            make(map[uint]MediaSource),
		diskReporters:           make(map[uint]DiskReporter),
		deleters:                make(map[uint]MediaDeleter),
		watchProviders:          make(map[uint]WatchDataProvider),
		requestProviders:        make(map[uint]RequestProvider),
		watchlistProviders:      make(map[uint]WatchlistProvider),
		ruleValueFetchers:       make(map[uint]RuleValueFetcher),
		collectionResolvers:     make(map[uint]CollectionResolver),
		collectionDataProviders: make(map[uint]CollectionDataProvider),
		labelDataProviders:      make(map[uint]LabelDataProvider),
		labelManagers:           make(map[uint]LabelManager),
		posterManagers:          make(map[uint]PosterManager),
	}
}

// Register adds an integration client to the registry, automatically detecting
// which capability interfaces it implements. The integrationID is the DB primary key.
//
// INVARIANT: If a client implements MediaSource, it should also implement MediaDeleter
// and DiskReporter. A MediaSource without MediaDeleter means items enter the evaluation
// pool but can never be deleted — this is a misconfiguration. Only *arr integrations
// (Sonarr, Radarr, Lidarr, Readarr) should implement MediaSource.
func (r *IntegrationRegistry) Register(integrationID uint, client interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	registered := 0

	if c, ok := client.(Connectable); ok {
		r.connectors[integrationID] = c
		registered++
	}
	if c, ok := client.(MediaSource); ok {
		// Warn if a MediaSource doesn't also implement MediaDeleter — likely a bug.
		if _, hasDeleter := client.(MediaDeleter); !hasDeleter {
			slog.Warn("Integration implements MediaSource but not MediaDeleter — items will enter evaluation but cannot be deleted",
				"component", "registry", "integrationID", integrationID)
		}
		r.mediaSources[integrationID] = c
		registered++
	}
	if c, ok := client.(DiskReporter); ok {
		r.diskReporters[integrationID] = c
		registered++
	}
	if c, ok := client.(MediaDeleter); ok {
		r.deleters[integrationID] = c
		registered++
	}
	if c, ok := client.(WatchDataProvider); ok {
		r.watchProviders[integrationID] = c
		registered++
	}
	if c, ok := client.(RequestProvider); ok {
		r.requestProviders[integrationID] = c
		registered++
	}
	if c, ok := client.(WatchlistProvider); ok {
		r.watchlistProviders[integrationID] = c
		registered++
	}
	if c, ok := client.(RuleValueFetcher); ok {
		r.ruleValueFetchers[integrationID] = c
		registered++
	}
	if c, ok := client.(CollectionResolver); ok {
		r.collectionResolvers[integrationID] = c
		registered++
	}
	if c, ok := client.(CollectionDataProvider); ok {
		r.collectionDataProviders[integrationID] = c
		registered++
	}
	if c, ok := client.(LabelDataProvider); ok {
		r.labelDataProviders[integrationID] = c
		registered++
	}
	if c, ok := client.(LabelManager); ok {
		r.labelManagers[integrationID] = c
		registered++
	}
	if c, ok := client.(PosterManager); ok {
		r.posterManagers[integrationID] = c
		registered++
	}

	slog.Debug("Registered integration", "component", "registry",
		"integrationID", integrationID, "capabilities", registered)
}

// Unregister removes all capability registrations for the given integration ID.
func (r *IntegrationRegistry) Unregister(integrationID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.connectors, integrationID)
	delete(r.mediaSources, integrationID)
	delete(r.diskReporters, integrationID)
	delete(r.deleters, integrationID)
	delete(r.watchProviders, integrationID)
	delete(r.requestProviders, integrationID)
	delete(r.watchlistProviders, integrationID)
	delete(r.ruleValueFetchers, integrationID)
	delete(r.collectionResolvers, integrationID)
	delete(r.collectionDataProviders, integrationID)
	delete(r.labelDataProviders, integrationID)
	delete(r.labelManagers, integrationID)
	delete(r.posterManagers, integrationID)
}

// Clear removes all registrations.
func (r *IntegrationRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connectors = make(map[uint]Connectable)
	r.mediaSources = make(map[uint]MediaSource)
	r.diskReporters = make(map[uint]DiskReporter)
	r.deleters = make(map[uint]MediaDeleter)
	r.watchProviders = make(map[uint]WatchDataProvider)
	r.requestProviders = make(map[uint]RequestProvider)
	r.watchlistProviders = make(map[uint]WatchlistProvider)
	r.ruleValueFetchers = make(map[uint]RuleValueFetcher)
	r.collectionResolvers = make(map[uint]CollectionResolver)
	r.collectionDataProviders = make(map[uint]CollectionDataProvider)
	r.labelDataProviders = make(map[uint]LabelDataProvider)
	r.labelManagers = make(map[uint]LabelManager)
	r.posterManagers = make(map[uint]PosterManager)
}

// ─── Accessor methods ───────────────────────────────────────────────────────

// Connector returns the Connectable for the given integration, or an error if not registered.
func (r *IntegrationRegistry) Connector(id uint) (Connectable, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("integration %d not registered as Connectable", id)
}

// MediaSources returns all registered MediaSource implementations with their IDs.
func (r *IntegrationRegistry) MediaSources() map[uint]MediaSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]MediaSource, len(r.mediaSources))
	for k, v := range r.mediaSources {
		out[k] = v
	}
	return out
}

// DiskReporters returns all registered DiskReporter implementations with their IDs.
func (r *IntegrationRegistry) DiskReporters() map[uint]DiskReporter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]DiskReporter, len(r.diskReporters))
	for k, v := range r.diskReporters {
		out[k] = v
	}
	return out
}

// Deleter returns the MediaDeleter for the given integration, or an error if not registered.
func (r *IntegrationRegistry) Deleter(id uint) (MediaDeleter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.deleters[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("integration %d not registered as MediaDeleter", id)
}

// WatchProviders returns all registered WatchDataProvider implementations with their IDs.
func (r *IntegrationRegistry) WatchProviders() map[uint]WatchDataProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]WatchDataProvider, len(r.watchProviders))
	for k, v := range r.watchProviders {
		out[k] = v
	}
	return out
}

// RequestProviders returns all registered RequestProvider implementations with their IDs.
func (r *IntegrationRegistry) RequestProviders() map[uint]RequestProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]RequestProvider, len(r.requestProviders))
	for k, v := range r.requestProviders {
		out[k] = v
	}
	return out
}

// WatchlistProviders returns all registered WatchlistProvider implementations with their IDs.
func (r *IntegrationRegistry) WatchlistProviders() map[uint]WatchlistProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]WatchlistProvider, len(r.watchlistProviders))
	for k, v := range r.watchlistProviders {
		out[k] = v
	}
	return out
}

// RuleValueFetcherFor returns the RuleValueFetcher for the given integration, or an error.
func (r *IntegrationRegistry) RuleValueFetcherFor(id uint) (RuleValueFetcher, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.ruleValueFetchers[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("integration %d not registered as RuleValueFetcher", id)
}

// CollectionResolver returns the CollectionResolver for the given integration, or false if not registered.
func (r *IntegrationRegistry) CollectionResolver(id uint) (CollectionResolver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collectionResolvers[id]
	return c, ok
}

// CollectionDataProviders returns all registered CollectionDataProvider implementations with their IDs.
func (r *IntegrationRegistry) CollectionDataProviders() map[uint]CollectionDataProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]CollectionDataProvider, len(r.collectionDataProviders))
	for k, v := range r.collectionDataProviders {
		out[k] = v
	}
	return out
}

// LabelDataProviders returns all registered LabelDataProvider implementations with their IDs.
func (r *IntegrationRegistry) LabelDataProviders() map[uint]LabelDataProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]LabelDataProvider, len(r.labelDataProviders))
	for k, v := range r.labelDataProviders {
		out[k] = v
	}
	return out
}

// LabelManagers returns all registered LabelManager implementations with their IDs.
// Returns a defensive copy. Used by SunsetService to apply/remove sunset labels.
func (r *IntegrationRegistry) LabelManagers() map[uint]LabelManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]LabelManager, len(r.labelManagers))
	for k, v := range r.labelManagers {
		out[k] = v
	}
	return out
}

// BuildTMDbToNativeIDMaps builds per-integration TMDb ID → media server native
// ID maps for all registered LabelManager-capable integrations (Plex, Jellyfin,
// Emby). Returns map[integrationID]map[tmdbID]nativeID. Each integration has
// its own map because the same TMDb ID resolves to different native IDs on
// different media servers (Plex ratingKey vs Jellyfin/Emby item ID).
//
// Used by SunsetService and PosterOverlayService to translate TMDb IDs from
// *arr items into the correct per-server identifiers for label and poster
// operations.
func (r *IntegrationRegistry) BuildTMDbToNativeIDMaps() map[uint]map[int]string {
	// Snapshot label manager IDs under lock, then release — the API calls
	// below are slow and must not hold the registry lock.
	r.mu.RLock()
	ids := make([]uint, 0, len(r.labelManagers))
	for id := range r.labelManagers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()

	result := make(map[uint]map[int]string, len(ids))
	for _, id := range ids {
		if plex, ok := r.PlexClient(id); ok {
			plexMap, err := plex.GetTMDbToRatingKeyMap()
			if err != nil {
				slog.Warn("Failed to build TMDb→RatingKey map for sunset",
					"component", "integrations", "integrationID", id, "error", err)
				continue
			}
			result[id] = plexMap
			continue
		}
		if jf, ok := r.JellyfinClient(id); ok {
			jfMap, err := jf.GetTMDbToItemIDMap()
			if err != nil {
				slog.Warn("Failed to build TMDb→ItemID map for sunset",
					"component", "integrations", "integrationID", id, "error", err)
				continue
			}
			result[id] = jfMap
			continue
		}
		if emby, ok := r.EmbyClient(id); ok {
			embyMap, err := emby.GetTMDbToItemIDMap()
			if err != nil {
				slog.Warn("Failed to build TMDb→ItemID map for sunset",
					"component", "integrations", "integrationID", id, "error", err)
				continue
			}
			result[id] = embyMap
			continue
		}
	}

	return result
}

// PosterManagers returns all registered PosterManager implementations with their IDs.
// Returns a defensive copy. Used by PosterOverlayService to upload/restore poster images.
func (r *IntegrationRegistry) PosterManagers() map[uint]PosterManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]PosterManager, len(r.posterManagers))
	for k, v := range r.posterManagers {
		out[k] = v
	}
	return out
}

// Connectors returns all registered Connectable implementations with their IDs.
func (r *IntegrationRegistry) Connectors() map[uint]Connectable {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[uint]Connectable, len(r.connectors))
	for k, v := range r.connectors {
		out[k] = v
	}
	return out
}

// TautulliClient checks if the Connectable at the given ID is a *TautulliClient
// and returns it. Used by the enricher builder since Tautulli doesn't implement
// WatchDataProvider (it uses per-item queries, not bulk).
func (r *IntegrationRegistry) TautulliClient(id uint) (*TautulliClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if tc, ok := c.(*TautulliClient); ok {
			return tc, true
		}
	}
	return nil, false
}

// PlexClient checks if the Connectable at the given ID is a *PlexClient
// and returns it. Used by the poller to build the TMDb→RatingKey map for
// Tautulli enrichment.
func (r *IntegrationRegistry) PlexClient(id uint) (*PlexClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if pc, ok := c.(*PlexClient); ok {
			return pc, true
		}
	}
	return nil, false
}

// JellystatClient checks if the Connectable at the given ID is a *JellystatClient
// and returns it. Used by the enricher builder since Jellystat doesn't implement
// WatchDataProvider (it requires a Jellyfin ID→TMDb ID map).
func (r *IntegrationRegistry) JellystatClient(id uint) (*JellystatClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if jc, ok := c.(*JellystatClient); ok {
			return jc, true
		}
	}
	return nil, false
}

// JellyfinClient checks if the Connectable at the given ID is a *JellyfinClient
// and returns it. Used by the poller to build JellyfinItemID→TMDbID maps for
// Jellystat enrichment.
func (r *IntegrationRegistry) JellyfinClient(id uint) (*JellyfinClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if jc, ok := c.(*JellyfinClient); ok {
			return jc, true
		}
	}
	return nil, false
}

// EmbyClient checks if the Connectable at the given ID is a *EmbyClient
// and returns it. Used by the poller to build Emby ItemID→TMDbID maps for
// Tracearr enrichment.
func (r *IntegrationRegistry) EmbyClient(id uint) (*EmbyClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if ec, ok := c.(*EmbyClient); ok {
			return ec, true
		}
	}
	return nil, false
}

// TracearrClient checks if the Connectable at the given ID is a *TracearrClient
// and returns it. Used by the enricher builder since Tracearr doesn't implement
// WatchDataProvider (it uses the TracearrEnricher with ID resolution maps).
func (r *IntegrationRegistry) TracearrClient(id uint) (*TracearrClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.connectors[id]; ok {
		if tc, ok := c.(*TracearrClient); ok {
			return tc, true
		}
	}
	return nil, false
}

// HasWatchProviders returns true if at least one WatchDataProvider is registered.
func (r *IntegrationRegistry) HasWatchProviders() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.watchProviders) > 0
}

// HasRequestProviders returns true if at least one RequestProvider is registered.
func (r *IntegrationRegistry) HasRequestProviders() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.requestProviders) > 0
}
