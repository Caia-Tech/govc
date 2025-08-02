# Changelog

All notable changes to govc will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note**: During alpha development (0.x.x), breaking changes may occur in minor version updates.

## [Unreleased]

### 🚧 Work in Progress
- Stash operations with untracked files
- Cluster state persistence
- Performance optimizations
- API stabilization

## [0.3.0] - 2024-08-02

### 🎯 Major Test Suite Fixes

This release focuses on stabilizing the test suite and fixing critical functionality issues identified through comprehensive testing.

### Added
- `DEVELOPMENT_STATUS.md` - Clear documentation about project's alpha status
- `TESTING_GUIDE.md` - Comprehensive testing documentation
- `isCommitHash()` helper function for better ref detection
- Empty snapshot support for time travel on new repositories
- Improved error messages throughout the codebase

### Changed
- **Storage Layer**:
  - Made `DeleteRef` operations idempotent across all implementations
  - Improved adapter pattern consistency
  - Enhanced concurrent access patterns
  
- **Repository Operations**:
  - Fixed stash to properly restore files in memory repositories
  - Enhanced checkout to support orphan branch creation
  - Improved branch detection for names with slashes
  
- **API Layer**:
  - Fixed repository deletion deadlocks
  - Updated tree listing type from "dir" to "directory"
  - Improved timestamp handling in tests

### Fixed
- ✅ TestStash - Now properly restores original content
- ✅ TestCheckoutSimple - Supports creating orphan branches
- ✅ TestTimeTravelSimple - Returns valid snapshot for empty repos
- ✅ TestRealWorldScenario - Eliminated deadlock issues
- ✅ Storage adapter tests - Consistent behavior across implementations
- ✅ RefManagerAdapter HEAD operations
- ✅ API timeout issues (408 errors)

### Technical Details
- Fixed memory repository exclusion in status calculations
- Resolved concurrent mutex access patterns
- Improved tree entry lookup performance
- Enhanced test determinism and reliability

## [0.2.0] - 2024-07-30

### Added
- JavaScript/TypeScript SDK (Phase 3.2)
- Go client library (Phase 3.1)
- Advanced features and integration preparation
- Comprehensive API test suite

### Changed
- Refactored storage architecture for better abstraction
- Improved API response consistency
- Enhanced error handling

### Fixed
- Multiple API compatibility issues
- Test failures in examples
- Branch operations with special characters

## [0.1.0] - 2024-07-15

### Added
- Initial memory-first Git implementation
- Basic repository operations (init, add, commit, branch, merge)
- RESTful API server
- Time travel functionality
- Parallel realities concept
- Authentication with JWT and API keys
- Prometheus metrics integration

### Known Issues
- Limited Git compatibility
- Performance not yet optimized
- Documentation incomplete

## Development Philosophy

govc follows these principles during development:

1. **Test-Driven**: Every fix includes comprehensive tests
2. **Incremental**: Small, focused releases
3. **Transparent**: All changes documented
4. **Community-Focused**: User feedback drives priorities

## Version Numbering

During alpha (0.x.x):
- **Patch** (0.0.x): Bug fixes, documentation updates
- **Minor** (0.x.0): New features, possible breaking changes
- **Major** (x.0.0): Reserved for stable release

---

For detailed commit history, see the [Git log](https://github.com/Caia-Tech/govc/commits/main).