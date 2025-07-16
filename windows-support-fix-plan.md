# Windows Support Fix Plan

This document outlines the steps needed to fix Windows support issues in the container-use repository tests and codebase.

## Current Issue Analysis

The main test failure is in `TestRepositoryOpen/valid_git_repository` with the error:
```
CreateFile C:\Users\zhichli\AppData\Local\Temp\TestRepositoryOpenvalid_git_repository1333234635\002\repos\C:\Users\zhichli\AppData\Local\Temp\TestRepositoryOpenvalid_git_repository1333234635\001: The filename, directory name, or volume label syntax is incorrect.
```

**Root Cause**: The `normalizeForkPath` function is creating invalid Windows paths by mixing `path.Join` (forward slashes) with `filepath.Join` (OS-aware) and not properly handling Windows absolute paths when constructing repository fork paths.

## Fix Plan

### ✅ Phase 1: Core Path Handling Issues

- [ ] **Fix `normalizeForkPath` function in `repository/git.go`**
  - Replace `path.Join` with `filepath.Join` on line 87
  - Fix Windows absolute path handling in lines 529 and 539
  - Ensure Windows drive letters (C:, D:, etc.) are properly escaped/normalized when used as subdirectories

- [ ] **Add Windows path validation and sanitization**
  - Create helper function to sanitize Windows paths for use as directory names
  - Handle Windows drive letters in repository paths (C:\ becomes C_ or similar)
  - Validate path lengths for Windows (260 character limit)

- [ ] **Update path construction throughout repository package**
  - Audit all uses of `path.Join` vs `filepath.Join` in repository package
  - Ensure consistent use of `filepath.Join` for file system operations
  - Check `getRepoPath()` and `getWorktreePath()` methods

### ✅ Phase 2: Test Infrastructure

- [ ] **Update repository tests for Windows compatibility**
  - Fix `TestRepositoryOpen` to handle Windows temporary directory paths
  - Add Windows-specific test cases for path handling
  - Ensure test cleanup works properly on Windows

- [ ] **Add Windows-specific integration tests**
  - Test repository operations with Windows paths containing spaces
  - Test long path scenarios (>260 characters)
  - Test drive letter handling in repository fork paths

### ✅ Phase 3: Configuration and Base Path Handling

- [ ] **Fix configuration directory handling**
  - Update `cuGlobalConfigPath` constant to use proper Windows paths
  - Implement Windows-aware config path resolution (use `%APPDATA%`)
  - Ensure `OpenWithBasePath` works correctly with Windows temp directories

- [ ] **Improve base path normalization**
  - Add function to normalize base paths for Windows
  - Handle tilde expansion properly on Windows
  - Ensure relative path resolution works correctly

### ✅ Phase 4: Git Command Integration

- [ ] **Audit git command execution for Windows**
  - Ensure git commands work with Windows paths
  - Check if any git commands need Windows-specific arguments
  - Verify proper error handling for Windows-specific git failures

- [ ] **Test Docker integration on Windows**
  - Verify Docker Desktop path handling
  - Test volume mounting with Windows paths
  - Ensure proper handling of Windows named pipes for Docker

### ✅ Phase 5: Error Handling and User Experience

- [ ] **Improve Windows error messages**
  - Add Windows-specific error messages for common issues
  - Provide better guidance for Docker Desktop requirements
  - Handle Windows permission errors gracefully

- [ ] **Add Windows documentation**
  - Update installation instructions for Windows
  - Document Windows-specific limitations or requirements
  - Add troubleshooting section for Windows users

### ✅ Phase 6: Build and CI/CD

- [ ] **Update build configuration**
  - Ensure proper Windows executable naming (.exe extension)
  - Update any scripts to handle Windows paths
  - Test cross-compilation for Windows

- [ ] **Add Windows testing to CI**
  - Add Windows runners to test suite
  - Test both PowerShell and cmd.exe environments
  - Verify installation scripts work on Windows

## Implementation Priority

1. **CRITICAL**: Fix the immediate test failure in `normalizeForkPath` (Phase 1, first checkbox)
2. **HIGH**: Complete Phase 1 path handling fixes
3. **HIGH**: Fix repository tests (Phase 2, first checkbox)  
4. **MEDIUM**: Complete remaining phases for comprehensive Windows support

## Testing Strategy

After each phase:
1. Run `go test ./repository` to verify repository package fixes
2. Run `go test ./...` to ensure no regressions in other packages
3. Test on actual Windows machine with Docker Desktop
4. Verify cross-platform compatibility still works on Unix systems

## Success Criteria

- [ ] All tests pass on Windows (`go test ./...`)
- [ ] Repository operations work with Windows paths
- [ ] Docker integration works with Docker Desktop
- [ ] Installation and setup work smoothly on Windows
- [ ] No regressions on Unix platforms
