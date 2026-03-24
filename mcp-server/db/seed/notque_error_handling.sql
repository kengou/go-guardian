-- Seed: notque error handling patterns ERR-1 through ERR-6
-- Source: notque knowledge base

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-1',
    'Bare Error Return: returning an error without wrapping it discards the call-site context. When the error surfaces, the stack trace is gone and the operation that failed is unknown.',
    'func readConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err // bare: caller sees "no such file or directory" with no path
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, err // bare: caller cannot tell this came from JSON decode
    }
    return &cfg, nil
}',
    'func readConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config %s: %w", path, err)
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config %s: %w", path, err)
    }
    return &cfg, nil
}',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-2',
    'String Error Comparison: comparing error values by their string representation is fragile — any rewording of the message silently breaks the check, and wrapped errors are never matched.',
    'func handleErr(err error) {
    if err.Error() == "not found" { // breaks if message changes or is wrapped
        return handle404()
    }
    if strings.Contains(err.Error(), "permission denied") { // fragile substring match
        return handle403()
    }
}',
    'var ErrNotFound = errors.New("not found")

func handleErr(err error) {
    if errors.Is(err, ErrNotFound) { // works through any wrapping chain
        return handle404()
    }
    var permErr *PermissionError
    if errors.As(err, &permErr) { // type-safe extraction
        return handle403()
    }
}',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-3',
    'Missing Sentinel Errors: constructing a new error value inline every time it is returned means callers cannot use errors.Is() to detect the specific condition — each call produces a distinct value that will never compare equal.',
    'func findUser(id int) (*User, error) {
    if id <= 0 {
        return nil, errors.New("not found") // new value each call — not comparable
    }
    // ...
}

// Caller:
if err.Error() == "not found" { ... } // forced into fragile string comparison',
    '// Package-level sentinel — one value, always the same pointer.
var ErrUserNotFound = errors.New("user not found")

func findUser(id int) (*User, error) {
    if id <= 0 {
        return nil, ErrUserNotFound
    }
    // ...
}

// Caller:
if errors.Is(err, ErrUserNotFound) { ... } // works through any wrapping',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-4',
    'Silent Error Suppression: discarding an error with the blank identifier silently hides failures. Callers and operators have no way to know the operation failed.',
    'func cleanup(path string) {
    _ = os.Remove(path) // failure silently ignored — disk full? permission denied?
}

func writeAudit(entry string) {
    _ = auditLog.Write([]byte(entry)) // dropped write goes unnoticed
}',
    'func cleanup(path string) error {
    if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
        return fmt.Errorf("cleanup %s: %w", path, err)
    }
    return nil
}

// When an error truly cannot be acted on, log it with enough context
// and add a comment explaining why it is safe to ignore.
func tryDeleteTemp(path string) {
    if err := os.Remove(path); err != nil {
        // best-effort: temp file will be cleaned by OS on reboot
        slog.Warn("delete temp file", "path", path, "err", err)
    }
}',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-5',
    'Re-wrapping Already-Wrapped Errors: adding another wrapping layer at the same level of abstraction produces redundant message chains like "save user: save user: ..." and makes errors harder to read.',
    'func saveUser(u User) error {
    if err := validateUser(u); err != nil {
        return fmt.Errorf("save user: %w", err) // first wrap
    }
    if err := db.Insert(u); err != nil {
        wrapped := fmt.Errorf("save user: %w", err) // second wrap — "save user: save user: ..."
        return fmt.Errorf("save user: %w", wrapped)
    }
    return nil
}',
    'func saveUser(u User) error {
    if err := validateUser(u); err != nil {
        // validateUser already wraps with its own context; just propagate.
        return err
    }
    if err := db.Insert(u); err != nil {
        return fmt.Errorf("save user %d: %w", u.ID, err) // wrap once, at this boundary
    }
    return nil
}',
    'notque',
    'error-handling'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'ERR-6',
    'Uppercase or Punctuated Error Messages: Go error strings are concatenated with fmt.Errorf. Starting with a capital letter or ending with a period produces malformed chains like "Failed to open file.: permission denied".',
    'func openConfig(path string) error {
    if err := os.Open(path); err != nil {
        return fmt.Errorf("Failed to open config file: %w.", err)
        // produces: "Failed to open config file: permission denied."
        //                              ^ capital          ^ trailing period
    }
    return nil
}',
    'func openConfig(path string) error {
    if _, err := os.Open(path); err != nil {
        return fmt.Errorf("open config %s: %w", path, err)
        // produces: "open config /etc/app.yaml: permission denied"
        //            ^ lowercase, no trailing period, contextual detail
    }
    return nil
}',
    'notque',
    'error-handling'
);
