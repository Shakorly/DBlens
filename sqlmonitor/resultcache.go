package main

import (
	"sync"
	"time"
)

// ResultCache preserves the last successfully collected value for every
// TierOnce and TierSlow field so that cycles which don't re-run those
// collectors still expose their data on the dashboard and in alerts.
//
// Think of it as "sticky memory" — once a field is populated it stays
// visible until a fresher collection replaces it.
type ResultCache struct {
	mu          sync.RWMutex
	inventory   cachedField[*ServerInventory]
	backups     cachedField[*BackupStatus]
	security    cachedField[*SecurityAudit]
	indexes     cachedField[*IndexMetrics]
	integrity   cachedField[*IntegrityMetrics]
	capacity    cachedField[*CapacityMetrics]
	sizes       cachedField[*SizeMetrics]
	jobs        cachedField[*JobMetrics]
	network     cachedField[*NetworkMetrics]
	queryStore  cachedField[*QueryStoreMetrics]
}

// cachedField holds a value and when it was last refreshed.
type cachedField[T any] struct {
	value     T
	updatedAt time.Time
	hasValue  bool
}

func (cf *cachedField[T]) set(v T) {
	cf.value = v
	cf.updatedAt = time.Now()
	cf.hasValue = true
}

func (cf *cachedField[T]) get() (T, bool) {
	return cf.value, cf.hasValue
}

// Absorb updates the cache with any non-nil fields from result.
func (rc *ResultCache) Absorb(r *CollectionResult) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if r.Inventory   != nil { rc.inventory.set(r.Inventory) }
	if r.Backups     != nil { rc.backups.set(r.Backups) }
	if r.Security    != nil { rc.security.set(r.Security) }
	if r.Indexes     != nil { rc.indexes.set(r.Indexes) }
	if r.Integrity   != nil { rc.integrity.set(r.Integrity) }
	if r.Capacity    != nil { rc.capacity.set(r.Capacity) }
	if r.Sizes       != nil { rc.sizes.set(r.Sizes) }
	if r.Jobs        != nil { rc.jobs.set(r.Jobs) }
	if r.Network     != nil { rc.network.set(r.Network) }
	if r.QueryStore  != nil { rc.queryStore.set(r.QueryStore) }
}

// Merge fills any nil fields in result with the last cached value.
// Fields that were freshly collected this cycle are left untouched.
func (rc *ResultCache) Merge(r *CollectionResult) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if r.Inventory  == nil { if v, ok := rc.inventory.get();  ok { r.Inventory  = v } }
	if r.Backups    == nil { if v, ok := rc.backups.get();    ok { r.Backups    = v } }
	if r.Security   == nil { if v, ok := rc.security.get();   ok { r.Security   = v } }
	if r.Indexes    == nil { if v, ok := rc.indexes.get();    ok { r.Indexes    = v } }
	if r.Integrity  == nil { if v, ok := rc.integrity.get();  ok { r.Integrity  = v } }
	if r.Capacity   == nil { if v, ok := rc.capacity.get();   ok { r.Capacity   = v } }
	if r.Sizes      == nil { if v, ok := rc.sizes.get();      ok { r.Sizes      = v } }
	if r.Jobs       == nil { if v, ok := rc.jobs.get();       ok { r.Jobs       = v } }
	if r.Network    == nil { if v, ok := rc.network.get();    ok { r.Network    = v } }
	if r.QueryStore == nil { if v, ok := rc.queryStore.get(); ok { r.QueryStore = v } }
}
