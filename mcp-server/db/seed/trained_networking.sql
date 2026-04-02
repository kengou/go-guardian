-- Seed: trained networking patterns NET-1 through NET-6
-- Source: Calico, Cilium

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-1',
    'Hash-Based eBPF Program Regeneration: recompiling eBPF programs on every policy change is expensive (clang invocation). Hash the config header, skip when unchanged, use revert stack for rollback on partial failure.',
    'func updateEndpoint(e *Endpoint) {
    compileBPF(e)  // expensive even if nothing changed
    loadBPF(e)
}',
    'newHash, _ := e.orchestrator.EndpointHash(e)
if newHash != e.bpfHeaderfileHash {
    datapathRegenCtxt.regenerationLevel = RegenerateWithDatapath
}
// On failure: revertStack.Revert() unwinds partial changes
// On success: finalizeList.Finalize() commits',
    'trained',
    'networking'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-2',
    'Versioned BPF Map Migration: unversioned BPF map names break on schema changes, requiring full dataplane restart. Version map pin names, detect size mismatch, run upgrade callback for live entry migration.',
    'map := bpf.CreateMap("my_map", keySize, valueSize)
// Schema change breaks live map, requires restart',
    'b := maps.NewPinnedMap(MapParams) // Name: "cali_v4_map_v2"
b.UpgradeFn = maps.Upgrade
// On size mismatch: old -> _old suffix, create new, migrate entries',
    'trained',
    'networking'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-3',
    'Multi-Index Policy Cache: O(policies * endpoints) evaluation on every change does not scale. Maintain indexed selector caches by namespace, resource, and label. RWMutex for reads, atomic revision for lock-free version checks.',
    'func onPolicyChange() {
    for _, ep := range allEndpoints {
        for _, pol := range allPolicies {
            if pol.Selects(ep) { apply(pol, ep) }
        }
    }
}',
    'rules     map[ruleKey]*rule
rulesByNS map[string]sets.Set[ruleKey]
rulesByRes map[ResourceID]map[ruleKey]*rule
revision   atomic.Uint64
// Read: RLock for concurrent queries
// Write: exclusive Lock, bump revision',
    'trained',
    'networking'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-4',
    'Strategy-Pattern IPAM: hard-coded IP allocation breaks across cloud providers. Strategy pattern with Allocator interface supports Kubernetes, MultiPool, CRD, ENI, Azure, GCP backends transparently.',
    'func allocateIP() net.IP {
    return hostLocalPool.Next() // only one strategy
}',
    'type Allocator interface {
    Allocate(pool Pool, owner string) (net.IP, error)
    Release(ip net.IP) error
}
// ConfigureAllocator selects backend by mode
// PoolOrDefault() fallback for callers without pool preference',
    'trained',
    'networking'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-5',
    'Batch BPF Map Iteration with Serialized Deletion: deleting BPF entries during iteration restarts the walk (O(n^2)). Batch iterate to collect expired entries, then serialize deletions under mutex. Treat ErrKeyNotExist as success (LRU eviction).',
    'for k, v := range ctMap.Iter() {
    if expired(v) { ctMap.Delete(k) }
    // Restarts iterator on every delete
}',
    'iter := bpf.NewBatchIterator(&m.Map)
toDelete := collectExpired(iter)
globalMutex.Lock()
for _, entry := range toDelete {
    if err := ctMap.Delete(entry.Key); errors.Is(err, ebpf.ErrKeyNotExist) {
        continue // LRU evicted, not an error
    }
}
globalMutex.Unlock()',
    'trained',
    'networking'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'NET-6',
    'Fan-Out Watch Proxy: N-node cluster with N API server watch connections overwhelms etcd at scale. Aggregation proxy maintains single watch per resource type, fans out to all node agents with snapshot-and-delta protocol.',
    'func (a *Agent) Start() {
    a.informer = cache.NewInformer(a.client, ...) // N agents = N watches
}',
    'type Typha struct {
    watchers map[resourceType]*watchMultiplexer
}
func (t *Typha) Subscribe(ctx context.Context, rt resourceType) <-chan Event {
    return t.watchers[rt].AddSubscriber(ctx)
    // Single watch per resource, fanned out to all subscribers
}',
    'trained',
    'networking'
);
