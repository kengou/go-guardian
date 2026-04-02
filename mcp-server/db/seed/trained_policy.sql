-- Seed: trained policy patterns POL-1 through POL-5
-- Source: OPA, Kyverno, Crossplane

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'POL-1',
    'Rule Evaluation Without State Isolation: evaluating policy rules that can mutate shared context leaks side effects between rules. Kyverno uses checkpoint/restore, OPA uses transaction rollback to isolate rule evaluation.',
    'func evaluateRules(ctx *Context, rules []Rule) ([]Result, error) {
    var results []Result
    for _, rule := range rules {
        result, err := rule.Evaluate(ctx) // rule mutates ctx, affects next rule
        if err != nil {
            return nil, err
        }
        results = append(results, result)
    }
    return results, nil
}',
    'func evaluateRules(ctx *Context, rules []Rule) ([]Result, error) {
    var results []Result
    for _, rule := range rules {
        ctx.Checkpoint() // save state before rule
        result, err := rule.Evaluate(ctx)
        if err != nil {
            ctx.Restore() // rollback side effects
            return nil, err
        }
        ctx.Restore() // clean state for next rule
        results = append(results, result)
    }
    return results, nil
}',
    'trained',
    'policy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'POL-2',
    'Runtime-Only Policy Validation: validating policy documents only at evaluation time means invalid policies are discovered in production. OPA validates AST safety at write time, Kyverno validates variable locations and rule structure at admission time.',
    'func applyPolicy(policy *Policy, input interface{}) (*Result, error) {
    // Policy is validated only when evaluated — errors surface in production
    return evaluate(policy, input)
}',
    'func createPolicy(ctx context.Context, policy *Policy) error {
    // Validate at write time — reject invalid policies before they affect traffic
    if err := validateAST(policy.Rules); err != nil {
        return fmt.Errorf("policy validation: %w", err)
    }
    if err := checkVariableLocations(policy); err != nil {
        return fmt.Errorf("forbidden variable location: %w", err)
    }
    if err := checkRuleSafety(policy); err != nil {
        return fmt.Errorf("unsafe rule (unbound variables): %w", err)
    }
    return r.Create(ctx, policy)
}',
    'trained',
    'policy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'POL-3',
    'Variable Injection in Policy Artifacts: parsing JSON patches or policy documents with unresolved variables allows injection through variable interpolation. Kyverno neutralizes variables with placeholders before parsing structure.',
    'func applyPatch(rawPatch []byte, variables map[string]string) ([]byte, error) {
    resolved := resolveVariables(rawPatch, variables)
    patch, err := jsonpatch.Decode(resolved) // variables could inject operations
    return patch.Apply(target)
}',
    'func applyPatch(rawPatch []byte, variables map[string]string) ([]byte, error) {
    // Step 1: validate structure with neutralized variables
    safePatch := replaceAllVars(rawPatch, func(s string) string {
        return "placeholder"
    })
    if _, err := jsonpatch.Decode(safePatch); err != nil {
        return nil, fmt.Errorf("invalid patch structure: %w", err)
    }
    // Step 2: resolve and apply only after structural validation
    resolved := resolveVariables(rawPatch, variables)
    patch, _ := jsonpatch.Decode(resolved)
    return patch.Apply(target)
}',
    'trained',
    'policy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'POL-4',
    'Unbounded Policy Evaluation Context: policy contexts that grow without bounds allow resource exhaustion via large policy inputs. Kyverno enforces a 2MB default cap on context size.',
    'func (ctx *Context) AddVariable(path string, value interface{}) error {
    data, _ := json.Marshal(value)
    ctx.data[path] = data // unbounded growth
    return nil
}',
    'const defaultMaxContextSize = 2 * 1024 * 1024 // 2MB

func (ctx *Context) AddVariable(path string, value interface{}) error {
    data, _ := json.Marshal(value)
    if ctx.size+len(data) > ctx.maxSize {
        return &ContextSizeLimitExceededError{
            Limit: ctx.maxSize,
            Size:  ctx.size + len(data),
        }
    }
    ctx.data[path] = data
    ctx.size += len(data)
    return nil
}',
    'trained',
    'policy'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'POL-5',
    'Default-Allow Capability Model: making all capabilities available by default means unreviewed features are accessible. OPA gates built-in functions by version — new capabilities default to unavailable until explicitly supported.',
    'func getBuiltins() []Builtin {
    return allBuiltins // everything available, including unreviewed new features
}',
    'func getBuiltins(targetVersion string) ([]Builtin, error) {
    idx := versionIndex[targetVersion]
    if idx == nil {
        return nil, fmt.Errorf("unsupported version: %s", targetVersion)
    }
    var available []Builtin
    for _, b := range allBuiltins {
        if idx.Supports(b.Name) {
            available = append(available, b)
        }
    }
    return available, nil // only reviewed, version-gated features
}',
    'trained',
    'policy'
);
