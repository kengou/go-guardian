-- Seed: notque testing patterns TEST-1 through TEST-6
-- Source: notque knowledge base

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-1',
    'Missing t.Helper() in Test Helpers: a test helper that does not call t.Helper() reports failure at the line inside the helper, not the line in the test that called it. This makes the test output misleading — the reader must chase the callstack to find the actual test.',
    'func assertEqual(t *testing.T, got, want int) {
    // t.Helper() is missing
    if got != want {
        t.Errorf("got %d, want %d", got, want)
        // line reported is inside here, not at the call site
    }
}',
    'func assertEqual(t *testing.T, got, want int) {
    t.Helper() // first line — redirects failure report to the caller
    if got != want {
        t.Errorf("got %d, want %d", got, want)
    }
}',
    'notque',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-2',
    'Non-Table-Driven Tests for Multiple Cases: writing a separate top-level function per test case causes duplication, inconsistent coverage, and makes adding new cases expensive. Table-driven tests with t.Run subtests are idiomatic Go.',
    'func TestAdd(t *testing.T) {
    if got := add(1, 2); got != 3 {
        t.Errorf("add(1,2) = %d, want 3", got)
    }
}

func TestAddNegative(t *testing.T) {
    if got := add(-1, -2); got != -3 {
        t.Errorf("add(-1,-2) = %d, want -3", got)
    }
}

func TestAddZero(t *testing.T) {
    if got := add(0, 0); got != 0 {
        t.Errorf("add(0,0) = %d, want 0", got)
    }
}',
    'func TestAdd(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name    string
        a, b    int
        want    int
    }{
        {"positive", 1, 2, 3},
        {"negative", -1, -2, -3},
        {"zeros", 0, 0, 0},
        {"mixed", -1, 1, 0},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            if got := add(tc.a, tc.b); got != tc.want {
                t.Errorf("add(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.want)
            }
        })
    }
}',
    'notque',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-3',
    'Internal Package Testing: placing tests in the same package as the code under test gives them access to unexported identifiers. This couples tests to implementation details and does not verify the public API that consumers actually use.',
    '// file: user/user.go
package user

// file: user/user_test.go
package user // same package — tests can access unexported fields

func TestUserInternal(t *testing.T) {
    u := User{internalField: "x"} // depends on unexported field
    // ...
}',
    '// file: user/user.go
package user

// file: user/user_test.go
package user_test // black-box: only the exported API is visible

import "yourmodule/user"

func TestUserPublicAPI(t *testing.T) {
    u, err := user.New("alice@example.com")
    // tests the contract consumers depend on
}

// Use package user (internal) only for whitebox tests of
// unexported helpers that genuinely cannot be tested via the public API.',
    'notque',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-4',
    'Missing t.Parallel() for Independent Tests: tests that share no mutable state run sequentially by default. Calling t.Parallel() on independent tests allows the go test runner to execute them concurrently, significantly reducing total test time.',
    'func TestFetchUser(t *testing.T) {
    // No shared state — but runs sequentially with all other tests.
    u, err := fetchUser(context.Background(), 1)
    if err != nil {
        t.Fatal(err)
    }
    // ...
}',
    'func TestFetchUser(t *testing.T) {
    t.Parallel() // safe: no shared mutable state
    u, err := fetchUser(context.Background(), 1)
    if err != nil {
        t.Fatal(err)
    }
    // ...
}

// Note: table-driven subtests should call t.Parallel() in both the
// parent test and each subtest, and loop variables must be captured.',
    'notque',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-5',
    'Old-Style Benchmark Loops: the traditional for i := 0; i < b.N; i++ loop requires manual reset timer calls around setup code. Go 1.24 introduced b.Loop() which handles this automatically and gives more accurate results.',
    'func BenchmarkProcess(b *testing.B) {
    data := setupData() // setup time incorrectly included in first iteration
    for i := 0; i < b.N; i++ {
        process(data)
    }
}

// With setup that should be excluded:
func BenchmarkProcessWithSetup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        data := setupData() // re-runs setup every iteration — timing noise
        b.ResetTimer()      // reset inside loop is incorrect usage
        process(data)
    }
}',
    'func BenchmarkProcess(b *testing.B) {
    data := setupData()
    for b.Loop() { // Go 1.24+: automatically excludes setup, no ResetTimer needed
        process(data)
    }
}

// b.Loop() also eliminates the need for b.StopTimer/b.StartTimer
// around per-iteration setup:
func BenchmarkProcessWithPerIterSetup(b *testing.B) {
    for b.Loop() {
        data := cheapSetup() // timed correctly per iteration
        process(data)
    }
}',
    'notque',
    'testing'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'TEST-6',
    'Code Generation for Mocks: generated mocks (mockgen, moq) require regeneration on every interface change and produce large files checked into source control. Manual mocks with function fields are simpler, more flexible, and need no tooling.',
    '//go:generate mockgen -source=service.go -destination=mock_service.go

// mock_service.go (generated — 80+ lines, requires go generate to update)
type MockService struct {
    ctrl *gomock.Controller
    // ...
}
func (m *MockService) EXPECT() *MockServiceMockRecorder { ... }
// ... many more generated methods',
    '// Manual mock: struct with function fields — one field per method.
type stubService struct {
    fetchUserFn func(ctx context.Context, id int) (*User, error)
}

func (s *stubService) FetchUser(ctx context.Context, id int) (*User, error) {
    if s.fetchUserFn != nil {
        return s.fetchUserFn(ctx, id)
    }
    return nil, nil
}

// Per-test customisation with no shared state:
func TestHandler_UserNotFound(t *testing.T) {
    t.Parallel()
    svc := &stubService{
        fetchUserFn: func(_ context.Context, _ int) (*User, error) {
            return nil, ErrUserNotFound
        },
    }
    h := NewHandler(svc)
    // ...
}',
    'notque',
    'testing'
);
