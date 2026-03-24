-- Seed: OWASP Top 10 baseline heuristic patterns for Go
-- Populates both owasp_findings and anti_patterns (source='owasp').
-- Categories follow OWASP Top 10 2021 identifiers.

-- ============================================================
-- A01 - Broken Access Control
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A01',
    '*.go',
    'Path traversal: http.FileServer or http.Dir used with a path derived from user-controlled input without sanitisation. An attacker can escape the intended directory with "../" sequences.',
    'Sanitise with filepath.Clean(path) and verify the result has the expected prefix using strings.HasPrefix. Serve only from a fixed, validated root. Never concatenate user input directly into file paths.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A01',
    '*.go',
    'Missing authorisation check: HTTP handler executes business logic or returns data without verifying the caller has permission to perform the action. No middleware or inline authz call is present before the handler body.',
    'Add an authorisation middleware (e.g. checking JWT claims or session role) that runs before every protected handler. For fine-grained checks, call an authz function as the first statement in the handler and return 403 on failure.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A01-1',
    'Path Traversal via http.Dir with User Input: passing a user-controlled path directly to http.Dir or os.Open without cleaning and prefix-checking allows directory traversal attacks.',
    'func serveFile(w http.ResponseWriter, r *http.Request) {
    // user controls r.URL.Path — could be "../../etc/passwd"
    http.ServeFile(w, r, "/var/www"+r.URL.Path)
}',
    'func serveFile(w http.ResponseWriter, r *http.Request) {
    root := "/var/www/static"
    // Clean collapses ".." components; HasPrefix enforces the root boundary.
    clean := filepath.Join(root, filepath.Clean("/"+r.URL.Path))
    if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    http.ServeFile(w, r, clean)
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A01-2',
    'Missing Authorisation Middleware: registering handlers without an authorisation layer allows any authenticated (or even unauthenticated) caller to invoke privileged operations.',
    'mux.HandleFunc("/admin/users", listUsersHandler)
mux.HandleFunc("/admin/delete", deleteUserHandler)
// No auth check — anyone who can reach the server can call these.',
    '// Wrap protected routes with an authz middleware.
protected := requireRole("admin")(mux)
adminMux := http.NewServeMux()
adminMux.HandleFunc("/admin/users", listUsersHandler)
adminMux.HandleFunc("/admin/delete", deleteUserHandler)

func requireRole(role string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims, ok := claimsFromContext(r.Context())
            if !ok || claims.Role != role {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}',
    'owasp',
    'security'
);

-- ============================================================
-- A02 - Cryptographic Failures
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A02',
    '*.go',
    'Weak hash for passwords: md5.New() or sha1.New() (or crypto/md5, crypto/sha1 imports) used in a context that handles passwords or sensitive credentials. MD5 and SHA-1 are broken for password hashing — they are fast, enabling brute-force attacks.',
    'Use crypto/bcrypt (bcrypt.GenerateFromPassword) or golang.org/x/crypto/argon2 for password storage. Never use MD5 or SHA-1 for anything security-sensitive.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A02',
    '*.go',
    'Hardcoded secret: string literals assigned to variables or constants named token, password, secret, apiKey, apiSecret, authToken, or privateKey. Secrets in source code are exposed in version control and build artefacts.',
    'Load secrets from environment variables (os.Getenv), a secrets manager (AWS Secrets Manager, HashiCorp Vault), or a mounted secret file. Never embed credential values in source code.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A02',
    '*.go',
    'TLS verification disabled: tls.Config with InsecureSkipVerify: true. Disabling certificate verification allows man-in-the-middle attacks on all TLS connections made with that configuration.',
    'Remove InsecureSkipVerify entirely. If a custom CA is needed, provide it via tls.Config.RootCAs. If the target uses self-signed certs in development, add the CA to the trust store — never disable verification in production builds.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A02-1',
    'Weak Hash for Password Storage: using MD5 or SHA-1 to hash passwords is cryptographically broken. Both are fast algorithms designed for data integrity, not password hardening.',
    'import "crypto/md5"

func hashPassword(pw string) string {
    h := md5.Sum([]byte(pw)) // NEVER use MD5 for passwords
    return hex.EncodeToString(h[:])
}',
    'import "golang.org/x/crypto/bcrypt"

func hashPassword(pw string) (string, error) {
    // Cost 12 is a reasonable default; tune upward as hardware improves.
    hash, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
    if err != nil {
        return "", fmt.Errorf("hash password: %w", err)
    }
    return string(hash), nil
}

func checkPassword(hash, pw string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A02-2',
    'Hardcoded Secrets in Source: embedding API keys, passwords, or tokens as string literals in Go source is exposed in version control history, compiled binaries, and container images.',
    'const apiKey = "sk-prod-abc123secret"   // hardcoded — visible in git log
const dbPassword = "hunter2"               // visible in compiled binary strings

func newClient() *Client {
    return &Client{key: apiKey}
}',
    'func newClient() (*Client, error) {
    key := os.Getenv("API_KEY")
    if key == "" {
        return nil, errors.New("API_KEY environment variable not set")
    }
    return &Client{key: key}, nil
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A02-3',
    'InsecureSkipVerify in TLS Config: setting InsecureSkipVerify: true disables all certificate chain and hostname verification, making TLS connections trivially interceptable.',
    'tr := &http.Transport{
    TLSClientConfig: &tls.Config{
        InsecureSkipVerify: true, // NEVER in production
    },
}
client := &http.Client{Transport: tr}',
    '// For custom CAs (e.g. internal PKI), add the CA cert instead.
caCert, err := os.ReadFile("/etc/ssl/certs/internal-ca.pem")
if err != nil {
    return nil, fmt.Errorf("load CA cert: %w", err)
}
pool := x509.NewCertPool()
pool.AppendCertsFromPEM(caCert)

tr := &http.Transport{
    TLSClientConfig: &tls.Config{
        RootCAs: pool, // trusted CA — no skip needed
    },
}
client := &http.Client{Transport: tr}',
    'owasp',
    'security'
);

-- ============================================================
-- A03 - Injection
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A03',
    '*.go',
    'SQL injection via fmt.Sprintf: SQL query strings built with fmt.Sprintf (or string concatenation) using values derived from HTTP request parameters, path variables, or any external input. An attacker can terminate the query and inject arbitrary SQL.',
    'Use parameterised queries exclusively: db.QueryContext(ctx, "SELECT ... WHERE id = $1", id). Never interpolate user input into SQL strings. Use a query builder or ORM that enforces parameterisation.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A03',
    '*.go',
    'Command injection via exec.Command with user-controlled arguments: os/exec.Command called with strings derived from user input. An attacker can inject shell metacharacters or additional command arguments.',
    'Never pass user input as a command argument without strict validation. Prefer whitelisting allowed values. Use exec.Command with a fixed binary path and individually validated arguments — never pass user input through a shell (sh -c).'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A03',
    '*.go',
    'XSS via template.HTML cast: html/template XSS protection bypassed by casting user-controlled strings to template.HTML, template.JS, or template.URL. This tells the template engine the value is already safe, suppressing escaping.',
    'Never cast user-provided strings to template.HTML or similar trusted types. Let html/template escape output automatically. If rich HTML is needed, use a sanitisation library (e.g. bluemonday) before casting.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A03-1',
    'SQL Injection via fmt.Sprintf: interpolating user-supplied values directly into SQL query strings is the most common and severe injection vulnerability.',
    'func getUser(db *sql.DB, username string) (*User, error) {
    // username comes from r.FormValue("user") — attacker controls it
    query := fmt.Sprintf("SELECT * FROM users WHERE name = ''%s''", username)
    row := db.QueryRow(query) // SQL injection
    // ...
}',
    'func getUser(ctx context.Context, db *sql.DB, username string) (*User, error) {
    // Parameterised query — driver handles escaping; SQL structure cannot change.
    row := db.QueryRowContext(ctx,
        "SELECT id, name, email FROM users WHERE name = $1",
        username,
    )
    var u User
    if err := row.Scan(&u.ID, &u.Name, &u.Email); err != nil {
        return nil, fmt.Errorf("get user %q: %w", username, err)
    }
    return &u, nil
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A03-2',
    'Command Injection via exec.Command with User Input: passing unsanitised user input as command arguments allows attackers to run arbitrary commands on the host system.',
    'func convertImage(filename string) error {
    // filename from HTTP upload — could be "foo.jpg; rm -rf /"
    cmd := exec.Command("sh", "-c", "convert "+filename+" output.png")
    return cmd.Run()
}',
    'func convertImage(filename string) error {
    // Whitelist: only allow alphanumeric names with a safe extension.
    if !regexp.MustCompile(`^[a-zA-Z0-9_-]+\.(jpg|png|gif)$`).MatchString(filename) {
        return errors.New("convert image: invalid filename")
    }
    // Pass as a distinct argument — no shell interpolation.
    cmd := exec.Command("convert", filepath.Join("/uploads", filename), "output.png")
    cmd.Stdout = io.Discard
    cmd.Stderr = io.Discard
    return cmd.Run()
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A03-3',
    'XSS via template.HTML Bypass: casting user-controlled strings to template.HTML disables html/template''s automatic escaping, allowing stored or reflected cross-site scripting.',
    'func renderProfile(w http.ResponseWriter, bio string) {
    // bio is user-supplied — could contain <script>alert(1)</script>
    data := struct{ Bio template.HTML }{Bio: template.HTML(bio)}
    tmpl.Execute(w, data) // XSS: bio rendered unescaped
}',
    'import "github.com/microcosm-cc/bluemonday"

func renderProfile(w http.ResponseWriter, bio string) {
    // Sanitise with a strict allowlist policy before any rendering.
    p := bluemonday.StrictPolicy()
    safeBio := p.Sanitize(bio)

    // Use plain string — html/template escapes any remaining special chars.
    data := struct{ Bio string }{Bio: safeBio}
    tmpl.Execute(w, data)
}',
    'owasp',
    'security'
);

-- ============================================================
-- A04 - Insecure Design
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A04',
    '*.go',
    'Missing input validation on handler parameters: HTTP handler reads query parameters, form values, or JSON body fields and uses them without validating length, type, format, or range. Attackers can send malformed or oversized inputs.',
    'Validate all inputs at the handler boundary: check required fields are present, enforce maximum lengths, validate formats (UUID, email, etc.) with regexp or a validation library. Return 400 Bad Request immediately on invalid input.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A04',
    '*.go',
    'Missing rate limiting on public endpoints: public-facing HTTP handlers have no rate limiting middleware. Endpoints are vulnerable to brute-force attacks (login, OTP), enumeration, and denial-of-service via request flooding.',
    'Add a rate limiter middleware (e.g. golang.org/x/time/rate token bucket, or a reverse-proxy level limiter) on all public endpoints. Apply stricter limits on authentication and sensitive data endpoints.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A04-1',
    'No Input Validation at Handler Boundary: consuming request values without validation means malformed, oversized, or malicious input reaches business logic and storage layers.',
    'func createUserHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name  string `json:"name"`
        Email string `json:"email"`
        Age   int    `json:"age"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    // name could be empty, 10 MB, or contain SQL — no check
    createUser(req.Name, req.Email, req.Age)
}',
    'func createUserHandler(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64 KB max body
    var req struct {
        Name  string `json:"name"`
        Email string `json:"email"`
        Age   int    `json:"age"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid JSON", http.StatusBadRequest)
        return
    }
    if req.Name == "" || len(req.Name) > 100 {
        http.Error(w, "name must be 1-100 chars", http.StatusBadRequest)
        return
    }
    if !emailRe.MatchString(req.Email) {
        http.Error(w, "invalid email", http.StatusBadRequest)
        return
    }
    if req.Age < 0 || req.Age > 150 {
        http.Error(w, "invalid age", http.StatusBadRequest)
        return
    }
    createUser(req.Name, req.Email, req.Age)
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A04-2',
    'Missing Rate Limiting on Public Endpoints: unrestricted endpoints can be brute-forced (passwords, OTP codes) or flooded with requests, consuming server resources.',
    'func loginHandler(w http.ResponseWriter, r *http.Request) {
    // No rate limit — attacker can try millions of passwords per second
    user, err := authenticate(r.FormValue("user"), r.FormValue("pass"))
    // ...
}',
    'import "golang.org/x/time/rate"

// Per-IP limiter: 5 requests/second, burst of 10.
var loginLimiter = newIPRateLimiter(rate.Every(200*time.Millisecond), 10)

func loginHandler(w http.ResponseWriter, r *http.Request) {
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    limiter := loginLimiter.get(ip)
    if !limiter.Allow() {
        http.Error(w, "too many requests", http.StatusTooManyRequests)
        return
    }
    user, err := authenticate(r.FormValue("user"), r.FormValue("pass"))
    // ...
}',
    'owasp',
    'security'
);

-- ============================================================
-- A05 - Security Misconfiguration
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A05',
    '*.go',
    'pprof imported in non-debug build: net/http/pprof imported without a build tag guard registers debug endpoints on the default HTTP mux. In production these endpoints expose heap dumps, goroutine stacks, CPU profiles, and timing information.',
    'Guard the pprof import with a build tag: // go:build debug. Never import net/http/pprof unconditionally in production code. Expose profiling endpoints on a separate internal-only port if needed.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A05',
    '*.go',
    'CORS AllowedOrigins wildcard in production: CORS middleware configured with AllowedOrigins: ["*"] or Origins("*") allows any web origin to make credentialed cross-origin requests, enabling cross-site request forgery and data exfiltration.',
    'Set AllowedOrigins to an explicit allowlist of known origins. Load the list from configuration so it can differ between environments. Never use "*" with AllowCredentials: true.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A05',
    '*.go',
    'Debug mode or verbose logging hardcoded to true: a debug or verbose flag hardcoded as true causes production deployments to emit detailed internal information (stack traces, SQL queries, internal IDs) to logs or HTTP responses accessible to users.',
    'Control debug/verbose mode via an environment variable or build tag. Default to false. Never hardcode debug: true or verbose: true in code that runs in production.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A05-1',
    'Unconditional pprof Import: importing net/http/pprof without a build tag registers /debug/pprof/* handlers on the default mux, exposing profiling data in production.',
    'import (
    "net/http"
    _ "net/http/pprof" // always registered — even in production
)

func main() {
    http.ListenAndServe(":8080", nil)
}',
    '// File: debug_pprof.go
//go:build debug

package main

import _ "net/http/pprof" // only included when built with -tags debug

// Production builds: go build .
// Debug builds:      go build -tags debug .',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A05-2',
    'Wildcard CORS in Production: allowing all origins bypasses the same-origin policy for every browser making requests to the service.',
    'corsMiddleware := cors.New(cors.Options{
    AllowedOrigins: []string{"*"}, // all origins allowed — unsafe in production
    AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
    AllowCredentials: true,        // wildcard + credentials is rejected by browsers AND dangerous
})',
    'allowedOrigins := strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")
// e.g. CORS_ALLOWED_ORIGINS=https://app.example.com,https://admin.example.com

corsMiddleware := cors.New(cors.Options{
    AllowedOrigins:   allowedOrigins, // explicit allowlist from config
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
    AllowCredentials: true,
    MaxAge:           300,
})',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A05-3',
    'Hardcoded Debug Flag: setting debug or verbose mode to true in source code causes production systems to leak internal details.',
    'var cfg = Config{
    Debug:   true,  // hardcoded — always on, even in production
    Verbose: true,
    LogLevel: "debug",
}',
    'cfg := Config{
    Debug:    os.Getenv("APP_DEBUG") == "true",
    Verbose:  os.Getenv("APP_VERBOSE") == "true",
    LogLevel: os.Getenv("LOG_LEVEL"),
}
if cfg.LogLevel == "" {
    cfg.LogLevel = "info" // safe default
}',
    'owasp',
    'security'
);

-- ============================================================
-- A07 - Identification and Authentication Failures
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A07',
    '*.go',
    'JWT signature not verified: jwt.ParseUnverified (or equivalent no-verify flag) used to parse a JWT token, or the parsed claims used without checking the err return from Parse. An attacker can forge any token payload.',
    'Always use jwt.Parse or jwt.ParseWithClaims with a proper keyfunc that returns the correct signing key. Verify the err return and reject the request if it is non-nil. Never use ParseUnverified on untrusted input.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A07',
    '*.go',
    'Missing CSRF token validation: HTTP handlers that perform state-changing operations (POST, PUT, DELETE, PATCH) on browser-accessible endpoints do not validate a CSRF token. Attackers can trick authenticated users into making unintended requests.',
    'Add CSRF middleware (e.g. github.com/gorilla/csrf) to all browser-facing state-changing endpoints. Verify the CSRF token on every non-idempotent request. APIs used only by non-browser clients should require a custom header (X-Requested-With) or use SameSite=Strict cookies.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A07-1',
    'JWT Parsed Without Signature Verification: using ParseUnverified or ignoring verification errors allows any client to forge arbitrary JWT claims.',
    'func getUserID(tokenStr string) (int, error) {
    // ParseUnverified skips ALL signature and claims validation
    token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
    claims := token.Claims.(jwt.MapClaims)
    return int(claims["sub"].(float64)), err
}',
    'var jwtKey = []byte(os.Getenv("JWT_SECRET"))

func getUserID(tokenStr string) (int, error) {
    token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return jwtKey, nil
    })
    if err != nil || !token.Valid {
        return 0, fmt.Errorf("invalid token: %w", err)
    }
    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return 0, errors.New("invalid claims")
    }
    sub, _ := claims["sub"].(float64)
    return int(sub), nil
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A07-2',
    'Missing CSRF Validation on State-Changing Endpoints: browser-accessible POST/PUT/DELETE handlers without CSRF token validation are vulnerable to cross-site request forgery.',
    'func deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
    // No CSRF check — a malicious page can trigger this for any logged-in user
    userID := sessionUserID(r)
    deleteUser(userID)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}',
    'import "github.com/gorilla/csrf"

// At server setup:
csrfMiddleware := csrf.Protect(
    []byte(os.Getenv("CSRF_KEY")),
    csrf.SameSite(csrf.SameSiteStrictMode),
)
mux.Use(csrfMiddleware)

func deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
    // csrf middleware has already validated the token before we get here.
    userID := sessionUserID(r)
    deleteUser(userID)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

// In forms: include {{ .CSRFField }} template function to emit the hidden input.',
    'owasp',
    'security'
);

-- ============================================================
-- A09 - Security Logging and Monitoring Failures
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A09',
    '*.go',
    'Sensitive fields logged: log.Printf, slog.Info, zap.Info, or similar log calls where the field name or format verb argument matches password, token, secret, key, authorization, or credential. Secrets written to logs are exposed to anyone with log access.',
    'Never log sensitive field values. Log only non-sensitive identifiers (user ID, request ID). If a field must be mentioned, log its presence or length, not its value: slog.Info("auth", "has_token", token != "").'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A09',
    '*.go',
    'Unstructured logging with log.Printf: using the standard library log package with Printf-style calls produces unstructured text logs that are difficult to query, parse, or correlate. Security events cannot be reliably extracted by SIEM tooling.',
    'Use log/slog (Go 1.21+) or a structured logger (zap, zerolog) with consistent field names. Log security events (authentication failures, authorisation denials, input validation failures) as structured records with user_id, request_id, ip, and event_type fields.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A09-1',
    'Logging Sensitive Fields: writing passwords, tokens, or secrets to any log output exposes credentials to anyone with log access, including log aggregation systems.',
    'func loginHandler(w http.ResponseWriter, r *http.Request) {
    user := r.FormValue("username")
    pass := r.FormValue("password")
    log.Printf("login attempt: user=%s password=%s", user, pass) // NEVER log passwords
    // ...
}',
    'func loginHandler(w http.ResponseWriter, r *http.Request) {
    user := r.FormValue("username")
    // Log the event and the user identity — never the credential.
    slog.Info("login attempt", "username", user, "ip", r.RemoteAddr)
    // ...
    if err := authenticate(user, r.FormValue("password")); err != nil {
        slog.Warn("login failed", "username", user, "ip", r.RemoteAddr)
        http.Error(w, "invalid credentials", http.StatusUnauthorized)
        return
    }
    slog.Info("login success", "username", user, "ip", r.RemoteAddr)
}',
    'owasp',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A09-2',
    'Unstructured Logging: log.Printf produces free-form text that cannot be reliably parsed, searched, or correlated by log aggregation and SIEM tools.',
    'log.Printf("user %d accessed resource %s at %v", userID, resource, time.Now())
log.Printf("ERROR: failed to connect to db: %v", err)',
    'logger := slog.Default()

logger.Info("resource accessed",
    "user_id", userID,
    "resource", resource,
    "request_id", requestID,
)

logger.Error("database connection failed",
    "error", err,
    "component", "db",
    "attempt", attempt,
)',
    'owasp',
    'security'
);

-- ============================================================
-- A10 - Server-Side Request Forgery (SSRF)
-- ============================================================

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A10',
    '*.go',
    'SSRF via http.Get with user-provided URL: http.Get, http.Post, or http.Client.Do called with a URL string derived from user input without host validation. An attacker can direct the server to fetch internal services, cloud metadata endpoints (169.254.169.254), or local files.',
    'Validate the URL before making any outbound request: parse with url.Parse, check the scheme (allow only https), resolve the hostname to IP addresses, and reject requests to private/loopback IP ranges (127.x, 10.x, 172.16-31.x, 192.168.x, 169.254.x). Maintain an allowlist of permitted hostnames where possible.'
);

INSERT OR IGNORE INTO owasp_findings (category, file_pattern, finding, fix_pattern)
VALUES (
    'A10',
    '*.go',
    'SSRF via url.Parse result used without host validation: url.Parse succeeds for any syntactically valid URL including file://, internal IPs, and cloud metadata addresses. Using the parsed URL directly in an HTTP call without checking the host allows SSRF.',
    'After url.Parse, explicitly validate: scheme is in an allowlist ([]string{"https"}), host does not resolve to a private/internal IP range, and host is on an allowlist if the application''s threat model requires it.'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'OWASP-A10-1',
    'SSRF via http.Get with User-Provided URL: making outbound HTTP requests to user-supplied URLs allows attackers to probe internal services, cloud metadata APIs, or local files.',
    'func fetchURL(w http.ResponseWriter, r *http.Request) {
    target := r.URL.Query().Get("url")
    // attacker can send: http://169.254.169.254/latest/meta-data/
    resp, err := http.Get(target)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    io.Copy(w, resp.Body)
}',
    'var allowedHosts = map[string]bool{
    "api.example.com":    true,
    "partner.service.io": true,
}

func fetchURL(w http.ResponseWriter, r *http.Request) {
    target := r.URL.Query().Get("url")
    u, err := url.Parse(target)
    if err != nil || (u.Scheme != "https") {
        http.Error(w, "invalid url", http.StatusBadRequest)
        return
    }
    host := u.Hostname()
    if !allowedHosts[host] {
        http.Error(w, "host not allowed", http.StatusForbidden)
        return
    }
    // Also resolve to IP and reject private ranges before dialling.
    if isPrivateHost(host) {
        http.Error(w, "host not allowed", http.StatusForbidden)
        return
    }
    resp, err := http.Get(u.String())
    // ...
}

// isPrivateHost resolves the hostname and checks against private IP ranges:
// 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16
func isPrivateHost(host string) bool {
    addrs, err := net.LookupHost(host)
    if err != nil {
        return true // treat unresolvable as private/invalid
    }
    private := []*net.IPNet{
        mustCIDR("127.0.0.0/8"),
        mustCIDR("10.0.0.0/8"),
        mustCIDR("172.16.0.0/12"),
        mustCIDR("192.168.0.0/16"),
        mustCIDR("169.254.0.0/16"),
        mustCIDR("::1/128"),
        mustCIDR("fc00::/7"),
    }
    for _, addr := range addrs {
        ip := net.ParseIP(addr)
        for _, block := range private {
            if block.Contains(ip) {
                return true
            }
        }
    }
    return false
}',
    'owasp',
    'security'
);
