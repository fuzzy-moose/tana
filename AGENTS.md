# Repository Guidelines

## Documentation

- Keep code and configuration as the source of truth for implemented behavior; document only rationale, conventions, and gotchas that cannot be inferred from them.
- Describe the current design on its own terms; omit documentation of removed behavior and comparisons with it.
- Update README files only when explicitly instructed or to correct contradictory or outdated information.

## Testing

- Test core behavior owned by this project; leave dependency logic to its own tests.
- Cover retained features only; omit tests for rejected or removed features.
- Accept coverage gaps for minor behavior. Keep tests proportional to impact; avoid complicating implementation solely to increase coverage.

## Agent skills

### Issue tracker
Local Markdown issues. Read `docs/agents/issue-tracker.md` before fetching, creating, or updating tickets.

### Triage labels
Default five-role vocabulary. Read `docs/agents/triage-labels.md` when triaging issues.

### Domain docs
Single-context layout. Read `docs/agents/domain.md` before exploring the codebase.
