-- Seed: notque go-anti-patterns skill — general patterns AP-1 through AP-7
-- Source: notque knowledge base

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-1',
    'Premature Interface Abstraction: defining an interface when only one implementation exists adds indirection with no benefit and obscures the real type from readers and tooling.',
    'type Storer interface {
    Store(ctx context.Context, item Item) error
}

type postgresStorer struct{ db *sql.DB }

func (p *postgresStorer) Store(ctx context.Context, item Item) error {
    _, err := p.db.ExecContext(ctx, "INSERT INTO items ...")
    return err
}

// Only one implementation — interface is noise.
func NewService(s Storer) *Service { return &Service{storer: s} }',
    'type postgresStorer struct{ db *sql.DB }

func (p *postgresStorer) store(ctx context.Context, item Item) error {
    _, err := p.db.ExecContext(ctx, "INSERT INTO items ...")
    return err
}

// Use concrete type directly.
// Extract the interface only when a second implementation (e.g. test fake,
// in-memory store) is genuinely needed.
func NewService(s *postgresStorer) *Service { return &Service{storer: s} }',
    'notque',
    'design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-2',
    'Goroutine Overkill: launching goroutines and synchronising with sync.WaitGroup for work that is purely sequential or CPU-bound adds scheduling overhead and obscures control flow without providing parallelism benefits.',
    'var wg sync.WaitGroup
results := make([]Result, len(items))
for i, item := range items {
    wg.Add(1)
    go func(idx int, it Item) {
        defer wg.Done()
        results[idx] = process(it) // CPU-bound, no I/O
    }(i, item)
}
wg.Wait()',
    '// Run sequentially for CPU-bound work.
// Introduce concurrency only after profiling shows a measurable win.
results := make([]Result, len(items))
for i, item := range items {
    results[i] = process(item)
}',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-3',
    'Error Wrapping Without Context: wrapping an error with a generic label like "error" or the function name alone discards the information needed to understand where and why the failure occurred.',
    'func loadConfig(path string) (*Config, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("error: %w", err)
    }
    // ...
}',
    'func loadConfig(path string) (*Config, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("load config from %s: %w", path, err)
    }
    // ...
}',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-4',
    'Channel Misuse: using channels to share a single variable between goroutines is semantically wrong — channels are for communication and coordination, not shared memory protection.',
    'ch := make(chan int, 1)
ch <- 0 // initial value

go func() {
    v := <-ch
    v++
    ch <- v
}()

v := <-ch
fmt.Println(v)',
    'var mu sync.Mutex
counter := 0

go func() {
    mu.Lock()
    counter++
    mu.Unlock()
}()

// Use channels only for goroutine signalling/coordination:
done := make(chan struct{})
go func() {
    defer close(done)
    doWork()
}()
<-done',
    'notque',
    'concurrency'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-5',
    'Generic Abuse: introducing type parameters for a function or type that only ever operates on a single concrete type creates unnecessary complexity and hinders readability without providing reuse.',
    'func Map[T any](items []T, fn func(T) T) []T {
    out := make([]T, len(items))
    for i, v := range items {
        out[i] = fn(v)
    }
    return out
}

// Called exactly once, always with []string — no other T is ever used.',
    '// Use a concrete type directly when only one type is needed.
func mapStrings(items []string, fn func(string) string) []string {
    out := make([]string, len(items))
    for i, v := range items {
        out[i] = fn(v)
    }
    return out
}

// Introduce generics only when the function is genuinely called with
// two or more distinct concrete types.',
    'notque',
    'design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-6',
    'Context Soup: threading context.Context through pure, side-effect-free functions that perform no I/O, issue no network calls, and have no cancellation semantics adds noise and signals a misunderstanding of context''s purpose.',
    'func add(ctx context.Context, a, b int) int {
    return a + b
}

func formatName(ctx context.Context, first, last string) string {
    return first + " " + last
}',
    '// Pure functions need no context.
func add(a, b int) int {
    return a + b
}

func formatName(first, last string) string {
    return first + " " + last
}

// Context belongs on functions that do I/O, RPCs, DB calls,
// or need cancellation/deadline propagation.
func fetchUser(ctx context.Context, id int) (*User, error) {
    return db.QueryRowContext(ctx, "SELECT ...", id)
}',
    'notque',
    'design'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-7',
    'Unnecessary Function Extraction: extracting a function that is only called once, contains trivial logic, and provides no genuine reuse merely to satisfy a cyclomatic complexity linter adds indirection without improving clarity.',
    'func processOrder(o Order) error {
    if err := validateOrderFields(o); err != nil { // called once, trivial
        return err
    }
    return saveOrder(o)
}

func validateOrderFields(o Order) error {
    if o.ID == "" {
        return errors.New("order id required")
    }
    return nil
}',
    'func processOrder(o Order) error {
    // Inline the validation — it is one check, called in one place.
    if o.ID == "" {
        return errors.New("process order: id required")
    }
    return saveOrder(o)
}

// Extract a function when:
//   - It is called in multiple places, OR
//   - It encapsulates a non-trivial algorithm that deserves its own tests.',
    'notque',
    'design'
);
