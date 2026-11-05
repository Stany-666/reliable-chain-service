# Reliable Chain Service Collaboration

## Sources of truth

- Rick's latest explicit message decides the immediate goal and whether he wants direct implementation.
- The repository, Git diff/log, tests, and reproducible runs decide engineering completion.
- This is a learning project. Rick writes the key code by default.

## Working together

- Before teaching or changing code, inspect relevant repository files, Git status, the current diff, and recent commits.
- Review unfinished working-tree changes before proposing a new feature.
- Start explanations with the business reason, then Ethereum data flow, backend design, and finally Go shape.
- Introduce at most two new concepts at a time.
- Unless Rick explicitly asks for implementation, provide explanation, a minimal skeleton or hint, review, and acceptance feedback instead of the finished implementation.
- Do not treat familiarity or committed code alone as proof of independent implementation ability.
- Preserve Rick's unrelated and pre-existing working-tree changes. Do not discard, rewrite, stage, or commit them implicitly.
- Do not modify Go, SQL, or configuration files merely because an audit found a problem.
- Do not expose credentials from `.env` or Git history. Refer to them only as redacted credentials.

## Validation and Git

- For code changes, run relevant checks and report exact results. The static baseline is `git diff --check`, `go test ./...`, and `go vet ./...`.
- Add integration, restart, fresh-setup, or race checks when the change requires them.
- Review business correctness, Go idiom, error handling, scope, and maintainability.
- Do not commit unless Rick explicitly requests the Git action.
