-- Seed: trained concurrency patterns CONC-7 through CONC-10
-- Source: Kubernetes, Prometheus, VictoriaMetrics, Greenhouse, VM Operator

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-7',
    'Missing HandleCrash in Controller Goroutines: a controller Run() method without panic recovery will crash the entire process if any worker panics. Every Kubernetes controller uses defer utilruntime.HandleCrash() at the top of Run().',
    'func (c *Controller) Run(ctx context.Context, workers int) {
    defer c.queue.ShutDown()

    for i := 0; i < workers; i++ {
        go wait.UntilWithContext(ctx, c.worker, time.Second)
    }
    <-ctx.Done()
}',
    'func (c *Controller) Run(ctx context.Context, workers int) {
    defer utilruntime.HandleCrash()
    defer c.queue.ShutDown()

    if !cache.WaitForNamedCacheSyncWithContext(ctx, c.listerSynced) {
        return
    }
    for i := 0; i < workers; i++ {
        go wait.UntilWithContext(ctx, c.worker, time.Second)
    }
    <-ctx.Done()
}',
    'trained',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-8',
    'Undocumented Lock Ordering: a struct with multiple mutexes and no documented acquisition order creates deadlock risk. Prometheus documents "mtx must not be taken after targetMtx" for its scrapePool.',
    'type Pool struct {
    mu       sync.Mutex
    targetMu sync.Mutex
    data     map[string]*Target
}

func (p *Pool) Update() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.targetMu.Lock() // which lock should be acquired first?
    defer p.targetMu.Unlock()
}',
    'type Pool struct {
    // mu protects data. Must not be held when acquiring targetMu.
    mu sync.Mutex
    // targetMu protects targets. May be acquired while holding mu,
    // but mu must not be acquired while holding targetMu.
    targetMu sync.Mutex
    data     map[string]*Target
}

func (p *Pool) Update() {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.targetMu.Lock()
    defer p.targetMu.Unlock()
}',
    'trained',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-9',
    'No Goroutine Leak Detection in Tests: test suites that spawn goroutines but do not check for leaks silently accumulate leaked goroutines. Prometheus and Kubernetes both use go.uber.org/goleak in TestMain.',
    'func TestMain(m *testing.M) {
    os.Exit(m.Run())
    // no leak detection — leaked goroutines go unnoticed
}',
    'func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
    // goleak fails the test if any unexpected goroutines remain
    // after all tests complete
}',
    'trained',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-10',
    'Controller Without Rate-Limited Requeue: a controller that requeues on every error without backoff creates a hot loop under persistent failures. Kubernetes uses TypedRateLimitingQueue with maxRetries. VM Operator classifies errors and applies custom rate limiters.',
    'func (c *Controller) handleErr(err error, key string) {
    if err == nil {
        c.queue.Forget(key)
        return
    }
    c.queue.Add(key) // immediate requeue — hot loop on persistent error
}',
    'func (c *Controller) handleErr(ctx context.Context, err error, key string) {
    if err == nil {
        c.queue.Forget(key)
        return
    }
    if c.queue.NumRequeues(key) < maxRetries {
        c.queue.AddRateLimited(key) // exponential backoff
        return
    }
    utilruntime.HandleError(err) // give up after maxRetries
    c.queue.Forget(key)
}',
    'trained',
    'concurrency'
);
