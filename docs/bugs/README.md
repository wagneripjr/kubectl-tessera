# Bug Reports

Bug reports live here as `BUG-NNN-slug.md` (zero-padded 3-digit sequential number, lowercase
hyphenated slug). Start at `BUG-001`.

When filing a bug:

1. Copy the template below into `docs/bugs/BUG-NNN-slug.md`.
2. Add a row to the **Bugs → Traceability** table in `../TRACEABILITY.md`.
3. Reference `BUG-NNN` in the fix commit message (e.g. `fix: resolve token clamp BUG-003`).
4. Update the bug's Status to `FIXED (YYYY-MM-DD)` and fill in Fix + Prevention when resolved.

Lifecycle: `OPEN → INVESTIGATING → FIXED (date) | WONT_FIX (date)`.

## Template

```markdown
# BUG-NNN: Short description

**Status**: OPEN
**Severity**: Critical | High | Medium | Low
**Found**: YYYY-MM-DD

## Related
- **Requirement**: FR-NNN / NFR-NNN (if applicable)
- **ADR**: ADR-NNN (if applicable)
- **Component**: affected internal/ package

## Symptoms
What is observed.

## Root Cause
Why it happens.

## Impact
Who/what is affected and how severely.

## Fix
What was changed.

## Test Gap
Why existing tests didn't catch this; the missing scenario.

## Prevention
The process/test change that prevents recurrence.
```

_No bugs filed yet._
