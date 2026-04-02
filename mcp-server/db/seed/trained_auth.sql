-- Seed: trained auth/identity patterns AUTH-1 through AUTH-6
-- Source: Vault, Zitadel, StackRox

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-1',
    'Deny-All Default Authorization: forgotten endpoints accessible when the default is allow. StackRox deny.Everyone() as default gRPC auth handler means missing authorization is a startup error, not a runtime exploit.',
    'func Authorized(ctx context.Context, method string) error {
    user := getUserFromCtx(ctx)
    if user != nil && user.HasRole("admin") {
        return nil
    }
    return nil // falls through to allow
}',
    'func (d *denyAll) Authorized(ctx context.Context, method string) error {
    return errox.NoAuthzConfigured.CausedBy("denies all access")
}
// Every endpoint must explicitly declare its authorizer
// Missing authz = startup error, not runtime exploit',
    'trained',
    'auth'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-2',
    'Composable Authorization Combinators: nested if/else chains for authorization are fragile and hard to audit. Use Authorizer interface with Or/And combinators for declarative, testable per-RPC authorization.',
    'func checkAccess(ctx context.Context, method string) error {
    if isAdmin(ctx) || isSensor(ctx) || isScanner(ctx) {
        return nil
    }
    if isUser(ctx) && method == "/v1/GetAlerts" {
        return nil
    }
    return errors.New("denied")
}',
    'var alertAuthz = perrpc.FromMap(map[string]authz.Authorizer{
    "/v1.AlertService/GetAlert": or.Or(
        user.With(permissions.View(resources.Alert)),
        idcheck.SensorOnly(),
    ),
    "/v1.AlertService/DeleteAlert": and.And(
        user.Authenticated(),
        user.With(permissions.Modify(resources.Alert)),
    ),
})',
    'trained',
    'auth'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-3',
    'Multi-Stage Token Validation: checking only token existence misses entity orphans, CIDR violations, and brute-force lockouts. Vault uses 4-stage pipeline: existence, entity consistency, CIDR binding, user lockout. Each stage fails closed independently.',
    'func validateToken(ctx context.Context, token string) (*User, error) {
    user, err := lookupToken(ctx, token)
    if err != nil {
        return nil, err
    }
    return user, nil // only checks existence
}',
    'te, err := c.tokenStore.Lookup(ctx, req.ClientToken)
if err != nil { return nil, nil, ErrInternalError }
if te == nil { return nil, nil, ErrPermissionDenied }
if te.EntityID != "" && entity == nil {
    return nil, te, ErrPermissionDenied
}
if !validateCIDR(req.Connection.RemoteAddr, te.BoundCIDRs) {
    return nil, nil, ErrPermissionDenied
}
if isUserLocked(ctx, entry, req) {
    return nil, nil, ErrPermissionDenied
}',
    'trained',
    'auth'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-4',
    'Chain-of-Responsibility Authentication: hard-coded auth type detection with nested if/else is rigid. IdentityExtractor chain returns (nil,nil) for not my type, (nil,error) for my type but invalid. Extractors compose without modifying each other.',
    'func authenticate(ctx context.Context) (*Identity, error) {
    token := extractBearer(ctx)
    if token != "" { return validateJWT(token) }
    cert := extractClientCert(ctx)
    if cert != nil { return validateCert(cert) }
    return nil, errors.New("unauthenticated")
}',
    'type IdentityExtractor interface {
    IdentityForRequest(ctx context.Context, ri RequestInfo) (Identity, error)
}
type extractorList []IdentityExtractor
func (l extractorList) IdentityForRequest(ctx context.Context, ri RequestInfo) (Identity, error) {
    for _, ext := range l {
        if id, err := ext.IdentityForRequest(ctx, ri); id != nil || err != nil {
            return id, err
        }
    }
    return nil, nil
}',
    'trained',
    'auth'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-5',
    'Envelope Encryption with Seal/Unseal: encrypting data directly with a master key means key rotation requires re-encrypting all data. Envelope encryption wraps per-secret DEKs with the master KEK. Rotation only re-wraps DEKs.',
    'func encrypt(data []byte, masterKey []byte) ([]byte, error) {
    block, _ := aes.NewCipher(masterKey)
    return gcmEncrypt(block, data)
    // Key rotation requires re-encrypting ALL data
}',
    'func encrypt(data []byte, kek KEK) (*EncryptedBlob, error) {
    dek := generateDEK()
    encData := gcmEncrypt(dek, data)
    wrappedDEK := kek.Wrap(dek)
    return &EncryptedBlob{Data: encData, WrappedKey: wrappedDEK}, nil
    // Key rotation only re-wraps DEKs, not the data
}',
    'trained',
    'auth'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AUTH-6',
    'Event-Sourced Identity: mutable identity state with direct DB updates loses audit history. Append-only event log with projections provides immutable audit trail by construction. Use for compliance-required identity systems.',
    'func updateEmail(userID, email string) error {
    _, err := db.Exec("UPDATE users SET email = ? WHERE id = ?", email, userID)
    return err // change history lost
}',
    'func changeEmail(userID, email string) error {
    return es.Push(ctx, &user.HumanEmailChangedEvent{
        UserID: userID,
        Email:  email,
    })
    // Current state projected from event stream
    // Immutable audit trail by construction
}',
    'trained',
    'auth'
);
