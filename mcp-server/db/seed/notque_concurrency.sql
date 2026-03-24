-- Seed: notque concurrency patterns CONC-1 through CONC-6
-- Source: notque knowledge base

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-1',
    'Goroutine Without Exit Path: launching a goroutine that has no mechanism to be stopped (no done channel, no context cancellation) creates a goroutine leak — the goroutine runs until the process exits and cannot be reclaimed.',
    'func startPoller(db *sql.DB) {
    go func() {
        for {
            poll(db) // runs forever; no way to stop it
            time.Sleep(5 * time.Second)
        }
    }()
}',
    'func startPoller(ctx context.Context, db *sql.DB) {
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return // clean exit when context cancelled
            case <-ticker.C:
                poll(db)
            }
        }
    }()
}',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-2',
    'Closing Channel From Receiver: a receiver closing the channel it reads from violates the ownership contract. If the sender writes after the close, the program panics. The sender — the owner — must be the one to close.',
    'func consume(ch chan int) {
    for v := range ch {
        process(v)
    }
    close(ch) // WRONG: receiver must not close the channel
}

func produce(ch chan int) {
    for i := 0; i < 10; i++ {
        ch <- i
    }
    // never reaches close — produce will panic or block
}',
    'func produce(ch chan int) {
    defer close(ch) // sender closes after it is done sending
    for i := 0; i < 10; i++ {
        ch <- i
    }
}

func consume(ch chan int) {
    for v := range ch { // range exits cleanly when channel is closed
        process(v)
    }
}',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-3',
    'Mutex Without Defer: manually calling Unlock without defer means a panic or early return in the critical section will leave the mutex permanently locked, deadlocking all future callers.',
    'func (s *Store) Set(key string, val int) {
    s.mu.Lock()
    if val < 0 {
        s.mu.Unlock() // easy to forget on every return path
        return
    }
    s.data[key] = val
    s.mu.Unlock()
}',
    'func (s *Store) Set(key string, val int) {
    s.mu.Lock()
    defer s.mu.Unlock() // always released, regardless of return path or panic
    if val < 0 {
        return
    }
    s.data[key] = val
}',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-4',
    'Arbitrary Channel Buffer Size: choosing a buffer size like 100 or 1000 by gut feel rather than analysis means the buffer either has no effect on backpressure or silently hides a slow consumer.',
    '// Why 100? No documented reason. Could be 1 or 10000.
jobs := make(chan Job, 100)

go func() {
    for j := range jobs {
        process(j) // what if this is slower than the producer?
    }
}()',
    '// Buffer size = number of workers, matching actual parallelism.
const numWorkers = 8
jobs := make(chan Job, numWorkers)

var wg sync.WaitGroup
for range numWorkers {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for j := range jobs {
            process(j)
        }
    }()
}

// Or use an unbuffered channel and let backpressure propagate naturally.
// Document the reasoning when a non-trivial buffer size is chosen.',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-5',
    'Missing select ctx.Done Case: a select loop that does not handle ctx.Done() cannot be cancelled. The goroutine leaks if the context is cancelled or if the channel it selects on never becomes ready.',
    'func worker(jobs <-chan Job) {
    for {
        select {
        case j := <-jobs:
            handle(j)
        // no ctx.Done — cannot be stopped
        }
    }
}',
    'func worker(ctx context.Context, jobs <-chan Job) {
    for {
        select {
        case <-ctx.Done():
            return // respect cancellation
        case j, ok := <-jobs:
            if !ok {
                return // channel closed
            }
            handle(j)
        }
    }
}',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'CONC-6',
    'WaitGroup Instead of errgroup: using sync.WaitGroup for concurrent work that can fail silently discards errors. Errors must be collected through ad-hoc channels or shared slices, which is error-prone. golang.org/x/sync/errgroup handles this correctly.',
    'var wg sync.WaitGroup
for _, url := range urls {
    wg.Add(1)
    go func(u string) {
        defer wg.Done()
        if err := fetch(u); err != nil {
            log.Println(err) // error is logged and lost — caller never sees it
        }
    }(url)
}
wg.Wait()
// no way for caller to know whether any fetch failed',
    'g, ctx := errgroup.WithContext(ctx)
for _, url := range urls {
    u := url
    g.Go(func() error {
        return fetch(ctx, u)
    })
}
if err := g.Wait(); err != nil {
    return fmt.Errorf("fetch batch: %w", err)
}',
    'notque',
    'concurrency'
);
