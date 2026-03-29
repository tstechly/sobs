# Release Notes

## Unreleased

### Added

- Logs page Query Statistics panel now supports query-scoped summaries (all matching rows across the active query, not only the visible page).
- Added a manual Run advanced analysis action on the Logs page for deeper message intelligence.
- Advanced analysis includes repeated message pattern fingerprints, error-family clustering, top keywords, and optimization hints.

### Changed

- Query statistics now stay consistent with grep filtering by evaluating stats on the same query result set used for rendering.

### Testing

- Added UI tests for query-scoped statistics behavior and manual advanced-analysis rendering.
