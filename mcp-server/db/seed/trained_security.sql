-- Seed: trained security patterns SEC-1 through SEC-10
-- Source: Cosign, Sealed-Secrets, OPA, Kyverno, Thanos, OTel Go

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-1',
    'Fail-Open Security Defaults: security decisions that allow access on error instead of denying. All major security projects (cosign, sealed-secrets, OPA, Kyverno) implement fail-closed behavior — verification errors block by default.',
    'func verifySignature(sig []byte) error {
    if err := verify(sig); err != nil {
        log.Warn("verification failed, allowing anyway")
        return nil // fail-open: attacker can bypass verification
    }
    return nil
}',
    'func verifySignature(sig []byte) error {
    if err := verify(sig); err != nil {
        return fmt.Errorf("signature verification failed: %w", err) // fail-closed
    }
    return nil
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-2',
    'Single-Key Decryption Without Rotation Support: decrypting with only the current key breaks when keys are rotated. Sealed-secrets iterates all available private keys before failing, supporting overlapping key validity periods.',
    'func decrypt(ciphertext []byte, key crypto.PrivateKey) ([]byte, error) {
    return tryDecrypt(ciphertext, key) // fails for data encrypted with previous key
}',
    'func decrypt(ciphertext []byte, keys []crypto.PrivateKey) ([]byte, error) {
    var lastErr error
    for _, key := range keys {
        plaintext, err := tryDecrypt(ciphertext, key)
        if err == nil {
            return plaintext, nil
        }
        lastErr = err
    }
    return nil, fmt.Errorf("decryption failed with all %d keys: %w", len(keys), lastErr)
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-3',
    'Missing Compile-Time Interface Assertion: security-critical types that must implement an interface should be verified at compile time with a blank identifier assignment. Cosign uses this for error types, crypto signers, and verifiers.',
    'type CosignError struct {
    Message string
    Code    int
}

func (e *CosignError) Error() string { return e.Message }
// No compile-time check — if Error() signature changes, fails at runtime',
    'type CosignError struct {
    Message string
    Code    int
}

var _ error = (*CosignError)(nil) // compile-time interface check

func (e *CosignError) Error() string { return e.Message }',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-4',
    'Scattered Environment Variable Access: using os.Getenv directly throughout the codebase makes it impossible to audit which env vars are read, which are sensitive, and what values are expected. Cosign registers all env vars centrally with metadata and bans direct os.Getenv via forbidigo.',
    'func getToken() string {
    return os.Getenv("API_TOKEN") // scattered, unauditable, sensitivity unknown
}',
    'var APIToken = mustRegisterEnv(Variable{
    Name:        "API_TOKEN",
    Description: "Authentication token for external API",
    Sensitive:   true,
    External:    false,
})

func getToken() string {
    return env.Getenv(APIToken) // centralized, auditable, sensitivity tracked
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-5',
    'Loading Private Keys Into Application Memory: when hardware tokens (PIV/YubiKey) are available, loading private key bytes into memory exposes them to memory dumps. Cosign delegates all crypto operations to the hardware device.',
    'func sign(data []byte, keyPath string) ([]byte, error) {
    keyBytes, _ := os.ReadFile(keyPath)
    privKey, _ := x509.ParsePKCS8PrivateKey(keyBytes) // key in memory
    return rsa.SignPKCS1v15(rand.Reader, privKey.(*rsa.PrivateKey), crypto.SHA256, hash)
}',
    'func sign(data []byte, card piv.Card, slot piv.Slot, cert *x509.Certificate) ([]byte, error) {
    // Private key never enters application memory
    privKey, err := card.PrivateKey(slot, cert.PublicKey, piv.KeyAuth{PIN: pin})
    if err != nil {
        return nil, fmt.Errorf("get hardware key: %w", err)
    }
    signer := privKey.(crypto.Signer)
    return signer.Sign(rand.Reader, hash, crypto.SHA256) // operation on hardware
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-6',
    'Verifying Certificates Against Local Clock: using time.Now() for certificate validity checks is vulnerable to clock skew and replay attacks. Cosign verifies against cryptographically attested transparency log timestamps.',
    'func verifyCert(cert *x509.Certificate) error {
    if time.Now().After(cert.NotAfter) {
        return fmt.Errorf("certificate expired") // local clock can be wrong
    }
    return nil
}',
    'func verifyCert(cert *x509.Certificate, tlogEntry *rekor.Entry) error {
    integratedTime := time.Unix(tlogEntry.IntegratedTime, 0)
    if integratedTime.Before(cert.NotBefore) || integratedTime.After(cert.NotAfter) {
        return fmt.Errorf("certificate not valid at signing time %v", integratedTime)
    }
    return nil
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-7',
    'Unjustified Cryptographic Shortcuts: using non-standard crypto patterns (zero nonce, weak hash) without documenting the architectural invariant that makes it safe. Sealed-secrets documents that zero nonce in AES-GCM is safe because each session key encrypts exactly one message.',
    'nonce := make([]byte, gcm.NonceSize()) // all zeros — why?
ciphertext := gcm.Seal(nil, nonce, plaintext, nil)',
    '// Zero nonce is safe here because sessionKey is random and encrypts exactly one message.
// Each RSA-OAEP envelope wraps a fresh 32-byte AES session key.
sessionKey := make([]byte, 32)
if _, err := rand.Read(sessionKey); err != nil {
    return nil, err
}
block, _ := aes.NewCipher(sessionKey)
gcm, _ := cipher.NewGCM(block)
ciphertext := gcm.Seal(nil, make([]byte, gcm.NonceSize()), plaintext, nil)',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-8',
    'Hardcoded Signing Implementation: using a fixed signing backend prevents key rotation to HSMs or remote signing services. OPA uses pluggable Signer/Verifier interfaces for bundle signing.',
    'func signBundle(data []byte, keyPath string) ([]byte, error) {
    key, _ := loadKey(keyPath) // hardcoded to file-based keys
    return sign(data, key)
}',
    'type Signer interface {
    Sign(data []byte) ([]byte, error)
}

type Verifier interface {
    Verify(data, signature []byte) error
}

// Register implementations for file, HSM, or remote signing
func signBundle(data []byte, signer Signer) ([]byte, error) {
    return signer.Sign(data) // pluggable backend
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-9',
    'Single-Layer Integrity Verification: verifying only the signature without checking the public key binding and content hash allows substitution attacks. Cosign checks signature match + public key match + SHA256 hash match independently.',
    'func verify(artifact []byte, sig []byte, pubKey crypto.PublicKey) error {
    return verifySignature(artifact, sig, pubKey) // only checks signature
}',
    'func verify(artifact []byte, sig []byte, bundle *Bundle) error {
    // Layer 1: signature matches content
    if err := verifySignature(artifact, sig, bundle.PublicKey); err != nil {
        return fmt.Errorf("signature mismatch: %w", err)
    }
    // Layer 2: public key matches expected identity
    if err := verifyKeyIdentity(bundle.PublicKey, bundle.CertChain); err != nil {
        return fmt.Errorf("key identity mismatch: %w", err)
    }
    // Layer 3: content hash matches manifest
    if err := verifyDigest(artifact, bundle.ExpectedDigest); err != nil {
        return fmt.Errorf("digest mismatch: %w", err)
    }
    return nil
}',
    'trained',
    'security'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'SEC-10',
    'Static Webhook Configuration: hardcoding admission webhook rules means they drift from actual policies. Kyverno dynamically generates webhook configurations from policy definitions + API discovery.',
    'func setupWebhook() *admissionv1.MutatingWebhookConfiguration {
    return &admissionv1.MutatingWebhookConfiguration{
        Webhooks: []admissionv1.MutatingWebhook{{
            Rules: []admissionv1.RuleWithOperations{{
                Rule: admissionv1.Rule{
                    APIGroups:   []string{"*"},
                    Resources:   []string{"*"}, // overly broad, or stale
                },
            }},
        }},
    }
}',
    'func (r *WebhookReconciler) buildWebhooks(ctx context.Context) (*admissionv1.MutatingWebhookConfiguration, error) {
    policies, err := r.listPolicies(ctx)
    if err != nil {
        return nil, fmt.Errorf("list policies: %w", err)
    }
    // Build rules from actual policy match conditions
    rules := make([]admissionv1.RuleWithOperations, 0)
    for _, p := range policies {
        rules = append(rules, policyToWebhookRule(p)...)
    }
    return &admissionv1.MutatingWebhookConfiguration{
        Webhooks: []admissionv1.MutatingWebhook{{Rules: rules}},
    }, nil
}',
    'trained',
    'security'
);
