-- Seed: trained distributed systems patterns DIST-1 through DIST-8
-- Source: etcd, ArgoCD

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-1',
    'Tracked Goroutine Lifecycle (GoAttach): fire-and-forget goroutines leak on shutdown and corrupt state. etcd GoAttach pattern rejects new goroutines during shutdown and tracks all active ones via WaitGroup.',
    'func (s *Server) startTask() {
    go func() {
        for {
            s.doWork()
            time.Sleep(time.Second)
        }
    }()
}',
    'func (s *Server) GoAttach(f func()) {
    s.wgMu.RLock()
    defer s.wgMu.RUnlock()
    select {
    case <-s.stopping:
        return
    default:
    }
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        f()
    }()
}',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-2',
    'Multi-Phase Graceful Shutdown: abrupt cancel+close crashes goroutines using resources. Ordered shutdown phases respect the resource dependency graph: signal stop, cancel contexts, wait goroutines, stop consensus, cleanup, signal done.',
    'func (s *Server) Stop() {
    s.cancel()
    s.conn.Close()
}',
    'func (s *Server) Stop() {
    s.wgMu.Lock()
    close(s.stopping)
    s.wgMu.Unlock()
    s.cancel()
    s.sched.Stop()
    s.wg.Wait()
    s.raft.Stop()
    s.Cleanup()
    close(s.done)
}',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-3',
    'Non-Blocking Event Fan-Out with Victim Tracking: blocking channel sends to slow consumers stall the entire event pipeline. Mark blocked watchers as victims, move to retry loop, keep main notification path fast.',
    'for _, watcher := range watchers {
    watcher.ch <- event // blocks if slow
}',
    'for w, eb := range newWatcherBatch(&s.synced, evs) {
    if w.send(WatchResponse{Events: eb.evs}) {
        pendingEventsGauge.Add(float64(len(eb.evs)))
    } else {
        w.victim = true
        victim[w] = eb
        s.synced.delete(w)
    }
}
s.addVictim(victim)',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-4',
    'Mutable vs Immutable Retry Classification: blindly retrying mutations risks duplicate writes. Classify operations as mutable (writes) vs immutable (reads). Retry reads on transient errors. Retry writes only when no endpoint was reachable.',
    'func retryRPC(fn func() error) error {
    for {
        if err := fn(); err != nil {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        return nil
    }
}',
    'func isSafeRetryMutableRPC(err error) bool {
    if ev, ok := status.FromError(err); ok && ev.Code() != codes.Unavailable {
        return false
    }
    desc := rpctypes.ErrorDesc(err)
    return desc == "there is no address available"
}

func isSafeRetryImmutableRPC(err error) bool {
    ev, ok := status.FromError(err)
    if !ok { return false }
    return ev.Code() == codes.Unavailable
}',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-5',
    'Watch Revision Tracking: restarting watches from the beginning reprocesses all events. Track last processed revision, resume from revision+1 on reconnect. Handle compaction errors by falling back to fresh list.',
    'watcher := client.Watch(ctx, prefix)
// Disconnects restart from beginning, reprocessing all events',
    'watcher := client.Watch(ctx, prefix, clientv3.WithRev(lastRevision+1))
// On compaction error: fall back to List + fresh Watch',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-6',
    'Lease-Based Ephemeral Ownership: polling for liveness is slow and unreliable. Use TTL-based leases with automatic expiration. Attach leases to ephemeral keys. Keep-alive goroutine renews while alive.',
    'func registerService(key, value string) {
    client.Put(ctx, key, value)
    go func() {
        for {
            time.Sleep(10 * time.Second)
            client.Put(ctx, key, value) // heartbeat
        }
    }()
}',
    'lease, _ := client.Grant(ctx, 15)
client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
ch, _ := client.KeepAlive(ctx, lease.ID)
go func() {
    for range ch { } // auto-renews until ctx cancelled
}()',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-7',
    'Atomic Hot-Path Counters: RWMutex on frequently-read state (applied index, term) creates contention under high concurrency. Use atomic.Uint64 for lock-free reads on the hot path.',
    'func (s *Server) AppliedIndex() uint64 {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.appliedIndex
}',
    'var appliedIndex atomic.Uint64

func (s *Server) setAppliedIndex(v uint64) {
    s.appliedIndex.Store(v)
}
func (s *Server) getAppliedIndex() uint64 {
    return s.appliedIndex.Load()
}',
    'trained',
    'distributed-systems'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DIST-8',
    'Linearizable Read via ReadIndex: reading local state in a distributed system may return stale data if the node is partitioned. ReadIndex confirms leadership with quorum before serving. Fall back to serializable when freshness is not required.',
    'func (s *Server) Get(key string) (string, error) {
    return s.localStore.Get(key) // may be stale if partitioned
}',
    'func (s *Server) Get(ctx context.Context, key string) (string, error) {
    if err := s.readIndex(ctx); err != nil {
        return "", fmt.Errorf("read index: %w", err)
    }
    return s.localStore.Get(key) // safe: leadership confirmed with quorum
}',
    'trained',
    'distributed-systems'
);
