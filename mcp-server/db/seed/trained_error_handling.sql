-- Seed: trained error handling patterns ERR-7 through ERR-10
-- Source: Kubernetes, Prometheus, Grafana, VictoriaMetrics, Perses

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-7',
    'Log-and-Return Error: logging an error and then returning it causes double-logging when the caller also logs. Kubernetes coding conventions explicitly forbid this. Let the caller decide how to handle the error.',
    'func process(ctx context.Context, key string) error {
    result, err := fetch(ctx, key)
    if err != nil {
        log.Error("failed to fetch", "key", key, "err", err)
        return fmt.Errorf("fetch %s: %w", key, err) // caller will log again
    }
    return nil
}',
    'func process(ctx context.Context, key string) error {
    result, err := fetch(ctx, key)
    if err != nil {
        return fmt.Errorf("fetch %s: %w", key, err) // caller decides to log
    }
    return nil
}

// When an error truly cannot be returned (event handler, goroutine callback),
// use utilruntime.HandleError instead of log + swallow:
func onEvent(obj interface{}) {
    if err := processEvent(obj); err != nil {
        utilruntime.HandleError(fmt.Errorf("process event: %w", err))
    }
}',
    'trained',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-8',
    'Error Wrapping With %s Instead of %w: using %s or %v to format an error inside fmt.Errorf breaks the error chain. errors.Is() and errors.As() cannot traverse the chain, making programmatic error handling impossible.',
    'func readFile(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read %s: %s", path, err) // %s loses chain
    }
    return data, nil
}

// Caller cannot use errors.Is:
if errors.Is(err, os.ErrNotExist) { // always false with %s
}',
    'func readFile(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read %s: %w", path, err) // %w preserves chain
    }
    return data, nil
}

// Caller can now use errors.Is:
if errors.Is(err, os.ErrNotExist) { // works through wrapping
    return nil // file does not exist — not an error
}',
    'trained',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-9',
    'Missing NotFound Check in Reconcilers: a controller reconciler that returns an error when the object is not found will be requeued forever for deleted objects. Every Kubernetes controller checks IsNotFound and returns nil.',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var obj appsv1.Deployment
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return ctrl.Result{}, err // requeues forever if object was deleted
    }
    // ...
}',
    'func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var obj appsv1.Deployment
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        if apierrors.IsNotFound(err) {
            return ctrl.Result{}, nil // object deleted — nothing to reconcile
        }
        return ctrl.Result{}, fmt.Errorf("get deployment %s: %w", req.NamespacedName, err)
    }
    // ...
}',
    'trained',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-10',
    'Panic in Library Code: using panic() in a package that is imported by other code prevents callers from recovering gracefully. Return errors instead. VictoriaMetrics uses Must* prefix functions that panic by convention, but this is a deliberate and documented choice.',
    'package mylib

func ParseConfig(data []byte) *Config {
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        panic(fmt.Sprintf("invalid config: %v", err))
    }
    return &cfg
}',
    'package mylib

func ParseConfig(data []byte) (*Config, error) {
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    return &cfg, nil
}

// If a Must* variant is needed (e.g., for init-time constants),
// name it clearly and document the panic behavior:
func MustParseConfig(data []byte) *Config {
    cfg, err := ParseConfig(data)
    if err != nil {
        panic(err) // intentional: only for program initialization
    }
    return cfg
}',
    'trained',
    'error-handling'
);
