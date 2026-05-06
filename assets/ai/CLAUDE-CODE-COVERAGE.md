# Comprehensive Test Coverage Prompt for Claude Code (Go)

## Objective

Achieve **95%+ statement coverage** through systematic, intelligent test development. Do not stop until this threshold is met. (Project-specific overrides may apply — cascade, for example, gates non-main/non-io packages at 100%.)

## Core Principles

### 1. Coverage is Non-Negotiable

- Target: **95%+ statement coverage** minimum
- **Never settle** for "good enough" at 70-80%
- Track progress explicitly: "Currently at X%, need Y% more"
- Continue iterating until threshold is met
- If you encounter obstacles, document them but **keep going**

### 2. Warnings Are There For A Reason

- Fixing warnings is a bug-prevention measure
- Deprecation warnings will bite you with breaks eventually (Go's compatibility promise covers the language and stdlib, not third-party packages)
- Warnings are a bad user experience and may erode confidence in the project
- Do NOT ignore warnings; when you see them, fix them
- You may temporarily set warning fix tasks to a lower priority, but you MUST return to them
- Treat `go vet` and `staticcheck` output as warnings of equal weight — both must be clean before a PR ships

### 3. Linting/Formatting Must Always Be Checked

- After each change, run `make format` (typically wraps `gofmt -w` / `goimports -w`)
- Before running new tests, ensure linting passes: `make lint` (typically wraps `go vet ./...` + `staticcheck ./...` + `golangci-lint run`)
- If the project doesn't have a Makefile, the canonical commands are:
  - `gofmt -d .` (verify) or `gofmt -w .` (apply)
  - `goimports -d .` or `goimports -w .` (manages imports + gofmt)
  - `go vet ./...`
  - `staticcheck ./...`
  - `golangci-lint run` (if configured)

### 4. Tests Must Pass — No Exceptions

- **Zero broken tests** is the only acceptable state
- A broken test indicates one of three things:
  1. The test is incorrectly written (fix the test)
  2. The implementation has a bug (fix the implementation)
  3. The test revealed an incorrect assumption (investigate and fix root cause)
- **Never** accept broken tests as "okay" or "not critical"
- **Never** call `t.Skip()` to hide failures, and **never** use a build tag (e.g. `//go:build skip_broken`) to exclude a test file just to make CI green

### 5. Fix Root Causes, Not Symptoms

When a test fails, follow this process:

**Step 1: Understand**

- Read the error message completely (run with `-v` if the failure isn't clear)
- Identify what the test expects vs. what actually happened
- Trace the execution path that led to the failure
- Ask: "What assumption is being violated?"

**Step 2: Diagnose**

- Is the test's expectation correct? (validate against spec/requirements)
- Is the implementation's behavior correct? (validate against intended design)
- Is there a mismatch in understanding?

**Step 3: Fix Intelligently**

- If the test is wrong: fix the test to match correct behavior
- If the implementation is wrong: fix the implementation bug
- If the design assumption is wrong: fix the design, then update both
- **Never** change implementation just to make a test pass without understanding why

**Anti-patterns to Avoid:**

- ❌ Changing return types to match test without understanding why
- ❌ Adding special cases in code just for tests
- ❌ Commenting out failing assertions or replacing `t.Errorf` with `t.Logf`
- ❌ Making tests less strict to avoid failures (e.g., dropping `cmp.Diff` for `if got == nil`)
- ✅ Understanding the contract, then ensuring both code and tests honor it

### 6. Systematic Coverage Approach

Follow this order:

**Phase 1: Package-by-Package Coverage**

```text
For each package in the module:
  1. Run coverage:    go test -coverprofile=coverage.out -covermode=atomic ./...
  2. Open report:     go tool cover -html=coverage.out -o coverage.html
  3. Identify uncovered statements in this package
  4. Write tests for uncovered code paths
  5. Verify tests pass: go test ./...
  6. Re-run coverage
  7. Repeat until package is 95%+ covered
```

**Phase 2: Integration Coverage**

```text
After all packages are covered:
  1. Check for uncovered integration paths (cross-package wiring)
  2. Write integration tests — typically as `_test.go` files in a companion
     `package foo_test` so they exercise the public API
  3. Verify all tests pass: go test ./...
  4. Re-run full coverage
```

**Phase 3: Edge Cases & Error Paths**

```text
Systematically test:
  - Error conditions (every returned error path)
  - Boundary values (empty slices, nil maps, zero-length strings, max values)
  - Nil-pointer / nil-interface inputs (typed-nil traps especially)
  - Concurrent access scenarios (run tests with -race)
  - Resource exhaustion (large inputs, deep recursion)
  - Invalid state transitions
  - Context cancellation and deadline expiration
```

### 7. Progress Tracking

After each testing session, report:

```text
Coverage Progress Report
========================
Current Coverage: X.X%
Target Coverage:  95.0%
Gap:              Y.Y%

Packages Completed (95%+):
- module/pkg_a: 98.5%
- module/pkg_b: 96.2%

Packages In Progress (<95%):
- module/pkg_c: 87.3% (needs: error path tests, edge cases)
- module/pkg_d: 72.1% (needs: full test suite)

Next Steps:
1. [Specific action for pkg_c]
2. [Specific action for pkg_d]

Blockers: [None | Describe any technical blockers]
```

## Testing Strategy by Code Type

The default Go testing idiom is **table-driven tests with subtests**. Use it everywhere unless there's a specific reason not to. Use `t.Helper()` on every assertion helper so failures point at the caller, not the helper. Use `t.Cleanup()` for teardown that should run even when the test fails. Use `t.Context()` (Go 1.24+) for a context scoped to the test, and `t.Chdir()` (Go 1.24+) for a test-scoped working directory.

### Pure Functions

Test a pure function with a single table-driven test and one subtest per case:

```go
// Test:
// - Happy path with typical inputs
// - Boundary values (0, 1, max, min, empty, nil)
// - Invalid inputs (if applicable)
// - Large inputs (stress test)

func TestParse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Config
        wantErr error
    }{
        {"happy path",    "key=value", Config{Key: "value"}, nil},
        {"empty input",   "",          Config{},             ErrEmpty},
        {"boundary max",  strings.Repeat("a=b\n", 1000), wantBig, nil},
        {"invalid",       "garbage",   Config{},             ErrMalformed},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := Parse(tc.input)
            if !errors.Is(err, tc.wantErr) {
                t.Fatalf("err = %v, want %v", err, tc.wantErr)
            }
            if diff := cmp.Diff(tc.want, got); diff != "" {
                t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
            }
        })
    }
}
```

### Functions with Side Effects

Use `t.TempDir()`, `t.Cleanup()`, and explicit fixture setup. `t.TempDir()` is automatically removed at the end of the test.

```go
// Test:
// - Expected side effects occur
// - Side effects are idempotent (if applicable)
// - Cleanup happens on error
// - State transitions are correct

func TestWriteAtomic(t *testing.T) {
    dir := t.TempDir()
    target := filepath.Join(dir, "out.txt")

    if err := WriteAtomic(target, []byte("hello")); err != nil {
        t.Fatalf("WriteAtomic: %v", err)
    }

    got, err := os.ReadFile(target)
    if err != nil {
        t.Fatalf("ReadFile: %v", err)
    }
    if string(got) != "hello" {
        t.Errorf("file content = %q, want %q", got, "hello")
    }
}

func TestWriteAtomic_Idempotent(t *testing.T) {
    dir := t.TempDir()
    target := filepath.Join(dir, "out.txt")

    for i := 0; i < 3; i++ {
        if err := WriteAtomic(target, []byte("v")); err != nil {
            t.Fatalf("attempt %d: %v", i, err)
        }
    }
}

func TestWriteAtomic_CleansUpOnError(t *testing.T) {
    dir := t.TempDir()
    // Make dir read-only to force a failure
    if err := os.Chmod(dir, 0o500); err != nil {
        t.Fatalf("chmod: %v", err)
    }
    t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

    target := filepath.Join(dir, "out.txt")
    if err := WriteAtomic(target, []byte("x")); err == nil {
        t.Fatal("expected error, got nil")
    }
    // Verify no half-written tempfile lingers
    entries, _ := os.ReadDir(dir)
    if len(entries) != 0 {
        t.Errorf("found %d leftover entries; want 0", len(entries))
    }
}
```

### Error Handling

Test EVERY error condition. Use `errors.Is` for sentinel comparison and `errors.As` for typed-error extraction. Never compare error messages with `strings.Contains`.

```go
// Test:
// - Each possible error variant
// - Error wrapping preserves the chain (errors.Is / errors.As both find it)
// - Cleanup on error

func TestLoad_Errors(t *testing.T) {
    tests := []struct {
        name    string
        path    string
        wantErr error
    }{
        {"file not found",     "/does/not/exist",      fs.ErrNotExist},
        {"permission denied",  "/root/forbidden",      fs.ErrPermission},
        {"invalid syntax",     "testdata/garbage.txt", ErrMalformed},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            _, err := Load(tc.path)
            if !errors.Is(err, tc.wantErr) {
                t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErr)
            }
        })
    }
}

func TestLoad_TypedError(t *testing.T) {
    _, err := Load("testdata/bad.json")
    var pe *json.SyntaxError
    if !errors.As(err, &pe) {
        t.Fatalf("expected *json.SyntaxError in chain; got %v", err)
    }
    if pe.Offset == 0 {
        t.Errorf("expected non-zero offset in syntax error")
    }
}
```

### Concurrent Code

Goroutines, channels, `sync.Mutex`/`sync.WaitGroup`, pipelines. Always run with `-race` in CI.

```go
// Test:
// - Happy path
// - Cancellation via context
// - Bounded lifetime: every goroutine started must terminate
// - No data races (run with: go test -race ./...)
// - Resource cleanup

func TestWorker_Cancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(t.Context()) // Go 1.24+
    defer cancel()

    done := make(chan struct{})
    go func() {
        defer close(done)
        Worker(ctx)
    }()

    cancel()
    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("worker did not exit after cancel")
    }
}

func TestPipeline_NoRace(t *testing.T) {
    // Implicitly verified when CI runs `go test -race ./...`.
    // Write the test as if races couldn't happen; the race detector catches them.
    in := make(chan int, 100)
    out := Pipeline(t.Context(), in)

    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            in <- i
        }
        close(in)
    }()

    var got []int
    for v := range out {
        got = append(got, v)
    }
    wg.Wait()

    if len(got) != 100 {
        t.Errorf("got %d outputs, want 100", len(got))
    }
}
```

### Time-Sensitive Code

Use `testing/synctest` (Go 1.25+) for deterministic virtual time. Never `time.Sleep` in tests waiting for "the right time" — that's flaky.

```go
// Test:
// - Time-based logic without real wall-clock waits
// - Timeouts and deadlines fire correctly
// - Periodic schedulers tick at the right times

func TestScheduler(t *testing.T) {
    synctest.Run(func() {
        s := NewScheduler()
        s.Start()
        t.Cleanup(s.Stop)

        time.Sleep(5 * time.Second) // virtual; returns instantly
        if got := s.Ticks(); got != 5 {
            t.Errorf("ticks after 5s = %d, want 5", got)
        }
    })
}
```

### State Machines

Table-drive the transitions. Test invalid transitions explicitly.

```go
// Test:
// - Every legal state transition
// - Every illegal transition (must error or stay put)
// - State persistence (if applicable)

func TestStateMachine_Transitions(t *testing.T) {
    tests := []struct {
        name      string
        from      State
        event     Event
        wantTo    State
        wantErr   error
    }{
        {"idle->running on Start",    StateIdle,    EventStart, StateRunning, nil},
        {"running->stopped on Stop",  StateRunning, EventStop,  StateStopped, nil},
        {"idle->? on Stop is invalid", StateIdle,    EventStop,  StateIdle,    ErrInvalidTransition},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            sm := &Machine{state: tc.from}
            err := sm.Apply(tc.event)
            if !errors.Is(err, tc.wantErr) {
                t.Fatalf("err = %v, want %v", err, tc.wantErr)
            }
            if sm.state != tc.wantTo {
                t.Errorf("state = %v, want %v", sm.state, tc.wantTo)
            }
        })
    }
}
```

## Handling Common Obstacles

### "This code is hard to test"

**Response:** Make it testable, don't skip it.

- Refactor for testability: take dependencies as interfaces, accept `io.Reader` instead of opening files internally, accept `context.Context` instead of using `context.Background()` deep in the call chain
- Use small, narrow interfaces at the consumer boundary (the canonical Go advice: "accept interfaces, return concrete types")
- Break large functions into smaller, testable units
- Use `httptest.NewServer` for HTTP clients, `fstest.MapFS` for filesystem readers
- **Document** why refactoring was needed in the commit message

### "This is just a wrapper / trivial"

**Response:** Test it anyway.

- Wrappers can have subtle bugs (wrong argument order, dropped errors, lost context cancellation)
- Tests document expected behavior
- The coverage tool counts these statements regardless
- It takes 60 seconds to write a trivial test

### "Coverage tool shows 100% but I don't believe it"

**Response:** Investigate the discrepancy.

- Verify all tests are actually running: `go test -v ./... | grep -c "^=== RUN"`
- Check for build-tag-excluded files: a `_test.go` behind `//go:build integration` won't run without `-tags=integration`
- Check for `t.Skip()` calls that always fire (use `go test -v` and look for `--- SKIP`)
- Remember: Go's coverage is **statement-level**, not branch-level. `if a && b` counts as one statement; you can have 100% statement coverage without testing all four boolean combinations. Read the code in addition to trusting the percentage.
- For richer analysis: `gocov` + `gocov-html`, `gcov2lcov` for codecov.io, or hand-audit `if`/`switch`/`for`/`select` branches against tests

### "Tests are slow"

**Response:** Optimize, don't skip.

- Identify the slow tests: `go test -v -count=1 ./... 2>&1 | awk '/--- (PASS|FAIL)/ {print $3, $2}' | sort -rn | head`
- Move slow tests behind a build tag: `//go:build integration` + a `make test-integration` target
- Or use `testing.Short()`: guard slow paths with `if testing.Short() { t.Skip("slow") }` and run the fast suite with `go test -short ./...`
- Mock expensive operations (network calls, real `exec.Command`) using interfaces
- Parallelize: add `t.Parallel()` to tests that don't share mutable state. Subtests inside `t.Run` can also be `t.Parallel()`.
- Cache: `go test` already caches passing tests by package; rerun with `-count=1` to bust the cache when needed

### "Can't reach this line"

**Response:** Understand why.

- Is it dead code? Remove it. (Run `staticcheck` — it'll often flag dead code as `U1000`.)
- Is it defensive programming? Test the defense by injecting the failure (return an error from a fake, pass a malformed input, force a context cancellation).
- Is it an unreachable error path because the underlying API can't actually fail? Document with a comment: `// Unreachable: bytes.Buffer.Write never returns a non-nil error.`
- If genuinely unreachable, exclude with a `// coverage:ignore` comment if your coverage tooling supports it, OR refactor so the unreachable branch goes away.

## Coverage Report Interpretation

When reading `go tool cover -html=coverage.out`:

**Green statements (covered):**

- ✅ Good, move on

**Red statements (uncovered):**

- 🔴 **MUST** be covered (unless legitimately unreachable — see above)
- Write a test that exercises this statement
- For statements inside compound conditions (`if a && b`), think about which input combinations actually reach the statement and ensure your tests hit them

**Gray statements (no count / not instrumented):**

- These are usually declarations, comments, or type definitions — not executable statements
- Verify with the source view that nothing important is being silently excluded

**Note on Go's coverage model:**

Go reports **statement-level** coverage by default. Branch coverage requires either reading the code or using third-party tooling. Be especially careful with:

```go
// Statement-level: 100% coverable with a single input.
// Branch-level: needs four inputs to fully exercise.
if condition_a && condition_b {
    do_something()
}

// Need tests for:
// - condition_a=true,  condition_b=true  (covers the call)
// - condition_a=true,  condition_b=false (covers the short-circuit on b)
// - condition_a=false, condition_b=*     (covers the short-circuit on a)
```

Compound conditions, `switch` statements with multiple cases, and `select` statements with multiple ready channels all need explicit per-branch test cases even when statement coverage shows green.

## Quality Gates

Before considering testing "complete":

- [ ] Overall coverage ≥ 95%
- [ ] All packages ≥ 90% coverage (no stragglers)
- [ ] All tests pass: `go test ./...` exits 0
- [ ] Race detector clean: `go test -race ./...` exits 0
- [ ] Vet clean: `go vet ./...` exits 0
- [ ] Static analysis clean: `staticcheck ./...` exits 0 (and `golangci-lint run` if configured)
- [ ] All error paths tested (every `if err != nil { return … }` has a test that hits it)
- [ ] All public APIs tested
- [ ] Integration tests exist (typically `package foo_test` in a separate file, exercising the public API)
- [ ] Edge cases covered (zero values, nil, empty, max sizes, boundary conditions)
- [ ] No `TODO` / `FIXME` in test code
- [ ] Tests are readable and maintainable
- [ ] Unit tests run in reasonable time (< 30s for the unit suite)
- [ ] Examples (`func ExampleX`) exist for non-trivial public APIs and are verified by `go test`

## Iterative Process

```text
WHILE coverage < 95%:
    1. Run:  go test -coverprofile=coverage.out -covermode=atomic ./...
    2. Open: go tool cover -html=coverage.out -o coverage.html  → view in browser
       (or:  go tool cover -func=coverage.out  for a text summary)
    3. Find lowest-covered package
    4. Identify specific uncovered statements
    5. Write tests for those statements
    6. Run:  go test ./...
    7. Fix any failures by understanding root cause
    8. Verify tests pass
    9. Re-run coverage
    10. Report progress

    If progress stalls for 2 iterations:
        - Analyze why (blockers, complexity, hard-to-mock dependency)
        - Refactor for testability if needed (split function, extract interface)
        - Ask for help/clarification if truly stuck
        - Document the blocker
        - **But keep trying alternative approaches**
END WHILE
```

## Anti-Patterns — Never Do These

❌ **Don't:** Skip tests because coverage is "high enough"
✅ **Do:** Continue until 95%+ threshold is met

❌ **Don't:** Mark failing tests with `t.Skip()` to make CI pass
✅ **Do:** Fix the root cause of the failure

❌ **Don't:** Change code to make a test pass without understanding why
✅ **Do:** Understand the failure, then fix correctly

❌ **Don't:** Write tests that don't actually test anything (just for coverage)
✅ **Do:** Write meaningful tests that validate behavior

❌ **Don't:** Test implementation details (private fields, exact log message strings, internal sequence of method calls)
✅ **Do:** Test behavior and contracts through the public API

❌ **Don't:** Compare errors with `strings.Contains(err.Error(), "not found")`
✅ **Do:** Use `errors.Is(err, ErrNotFound)` or `errors.As(err, &target)`

❌ **Don't:** `time.Sleep` in tests to wait for "the right moment"
✅ **Do:** Use channels, `synctest`, or `eventually(t, condition)` helpers

❌ **Don't:** Share mutable state across `t.Parallel()` tests
✅ **Do:** Each parallel subtest owns its own fixtures (`t.TempDir()`, fresh `*sql.DB` connection, etc.)

❌ **Don't:** Give up at 80% coverage
✅ **Do:** Push to 95%+ systematically

❌ **Don't:** Accept "close enough" on test assertions
✅ **Do:** Make assertions precise — `cmp.Diff` for structs, `errors.Is` for errors, exact equality for primitives

## Sample Test Development Session

```text
Step 1: Initial Coverage Check
$ go test -coverprofile=coverage.out -covermode=atomic ./...
ok    example.com/m/parser     0.123s  coverage: 67.3% of statements
ok    example.com/m/validator  0.045s  coverage: 72.0% of statements
ok    example.com/m/formatter  0.022s  coverage: 89.0% of statements

$ go tool cover -func=coverage.out | tail -1
total:    (statements)    67.3%

Step 2: Identify Gaps
$ go tool cover -html=coverage.out -o coverage.html
Opening coverage.html in browser...

Findings:
- example.com/m/parser:    45% (error paths not tested)
- example.com/m/validator: 72% (edge cases missing)
- example.com/m/formatter: 89% (close, needs a few tests)

Step 3: Start with Lowest
Working on: example.com/m/parser (45%)

Uncovered statements:
- parser.go:42-45  Error path when input is empty
- parser.go:78-82  Error path when syntax invalid
- parser.go:95-98  Handling of escaped characters

Step 4: Write Tests
Writing TestParse_EmptyInput, TestParse_InvalidSyntax, TestParse_EscapedChars
as cases in the existing TestParse table.

Step 5: Run Tests
$ go test ./parser
--- FAIL: TestParse/empty_input
    parser_test.go:34: err = <nil>, want errors.Is(_, ErrEmpty)
FAIL

Step 6: Debug Failure
Empty input falls through to the syntax checker first instead of being
caught early.

Root cause: Validation order is wrong in parser.go. Empty-input check
happens AFTER the syntax tokenizer attempts to consume the first byte
and reports a misleading "unexpected EOF" syntax error.

Step 7: Fix Root Cause
Move the empty-input check to the top of Parse() in parser.go:38.

Step 8: Verify Fix
$ go test ./parser
ok    example.com/m/parser    0.124s

Step 9: Check Progress
$ go test -coverprofile=coverage.out -covermode=atomic ./...
$ go tool cover -func=coverage.out | tail -1
total:    (statements)    71.2%

(was 67.3%, gained 3.9%)

Step 10: Continue
example.com/m/parser is now at 78%.
Still need: unicode edge cases, boundary conditions on token length.
Continuing...
```

## Final Reminder

**Your job is not done until:**

1. Coverage is ≥ 95%
2. All tests pass (including under `-race`)
3. All code paths are tested
4. You understand why every test exists
5. You understand why every test passes

**Persistence beats perfection.** Keep iterating, keep testing, keep improving.

## Success Criteria Checklist

At the end of testing work, verify:

```text
Testing Completion Checklist
============================
[ ] Coverage ≥ 95% overall (or project-specific threshold)
[ ] No package below 90% coverage
[ ] Zero failing tests
[ ] Zero skipped tests (except documented slow/integration tests behind build tags)
[ ] `go test -race ./...` passes
[ ] `go vet ./...` clean
[ ] `staticcheck ./...` clean
[ ] `golangci-lint run` clean (if configured)
[ ] All error paths tested (every returned error variant has a test)
[ ] All edge cases tested
[ ] All public APIs tested through `package foo_test` (external test package)
[ ] Examples (ExampleX functions) exist for non-trivial public APIs
[ ] Tests are documented and maintainable
[ ] CI/CD pipeline includes coverage checks
[ ] Coverage report reviewed and understood
[ ] All uncovered statements justified (with code comments if unreachable)

Total Test Count:        ___
Total Subtests:          ___
Test Execution Time:     ___ seconds (unit suite, without -race)
Race-Enabled Time:       ___ seconds (with -race)
Coverage:                ___%
```

**Sign off only when ALL boxes are checked.**

## Reference Commands

Quick reference for the canonical commands referenced throughout this prompt:

```bash
# Run all tests
go test ./...

# Run with race detector (use in CI)
go test -race ./...

# Run with verbose output
go test -v ./...

# Generate coverage profile
go test -coverprofile=coverage.out -covermode=atomic ./...

# Coverage summary (text)
go tool cover -func=coverage.out

# Coverage report (HTML, opens in browser)
go tool cover -html=coverage.out -o coverage.html

# Run only fast tests (those that respect testing.Short())
go test -short ./...

# Run a specific test by name pattern
go test -run TestParse ./parser

# Run integration tests behind a build tag
go test -tags=integration ./...

# Skip the test cache (force re-run)
go test -count=1 ./...

# Format and vet
gofmt -w .
goimports -w .
go vet ./...
staticcheck ./...
golangci-lint run
```
