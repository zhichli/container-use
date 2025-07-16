---
applyTo: '**'
---

# Windows Support Guidelines for container-use

This document provides specific guidelines for adding Windows support to the container-use CLI tool and MCP server.

## ✅ 1. Use Go's Cross-Platform Standard Library

Go's standard library is designed to be cross-platform. Always prefer it over shell commands or OS-specific syscalls.

**Examples:**
```go
// ✅ Good - Cross-platform
os.Mkdir(dir, 0755)       // → Creates directory with default Windows perms
os.Remove(file)           // → Works on all platforms
os.Rename(old, new)       // → Works on all platforms
filepath.Join("a", "b")   // → "a/b" (Unix), "a\\b" (Windows)

// ❌ Avoid - Platform-specific commands
exec.Command("mkdir", dir)  // → Only works on Unix
exec.Command("rm", file)    // → Only works on Unix
exec.Command("mv", old, new) // → Only works on Unix
```

## ✅ 2. Always Use filepath for File System Operations

The `path` package is for URL paths; `filepath` is OS-aware and handles Windows backslashes correctly.

```go
import "path/filepath" // Always use this for filesystem paths

// ✅ Correct
configPath := filepath.Join(os.UserHomeDir(), ".config", "container-use")
worktreePath := filepath.Join(basePath, "worktrees", envID)

// ❌ Wrong - Will break on Windows
configPath := path.Join(home, ".config/container-use") // Uses forward slashes only
```

## ✅ 3. Implement OS-Specific Code with Build Tags

Use build tags for platform-specific implementations. Follow the existing pattern in the codebase:

```go
// signal_windows.go
//go:build windows

package main

func setupSignalHandling() {
    // Windows-specific signal handling (SIGUSR1 not available)
}

// signal_unix.go  
//go:build !windows

package main

import "os/signal"

func setupSignalHandling() {
    // Unix signal handling with SIGUSR1
}
```

**Key patterns for container-use:**
- **Terminal/Console operations**: Windows uses `cmd.exe` vs Unix shells
- **Signal handling**: Windows doesn't support SIGUSR1, SIGTERM handling differs
- **File watching**: Different APIs for filesystem monitoring
- **Process management**: Windows process control differs from Unix

## ✅ 4. Windows-Specific Command Execution

For git operations and Docker commands, ensure Windows compatibility:

```go
// ✅ Good - Works cross-platform
func clearScreen() error {
    var cmd *exec.Cmd
    if runtime.GOOS == "windows" {
        cmd = exec.Command("cmd", "/c", "cls")
    } else {
        cmd = exec.Command("clear")
    }
    cmd.Stdout = os.Stdout
    return cmd.Run()
}

// ✅ Good - Git commands work the same
func runGitLog(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "git", "log", "--color=always", 
        "--remotes=container-use", "--oneline", "--graph", "--decorate")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

## ✅ 5. Handle Windows File System Differences

**File Permissions:**
Windows ignores Unix-style permissions (os.Chmod has limited effect):

```go
// ✅ Good - Use sensible defaults
file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)

// ❌ Don't rely on specific permissions working on Windows
if runtime.GOOS != "windows" {
    os.Chmod(file, 0755) // This is mostly ignored on Windows
}
```

**Path Length Limitations:**
Windows has a 260-character path limit (unless long path support is enabled):

```go
// ✅ Good - Check path lengths for Windows
func validatePath(path string) error {
    if runtime.GOOS == "windows" && len(path) > 250 {
        return fmt.Errorf("path too long for Windows: %s", path)
    }
    return nil
}
```

## ✅ 6. Docker Desktop Integration

container-use relies heavily on Docker. Ensure proper Docker Desktop detection:

```go
// ✅ Good - Enhanced Docker daemon error detection
func isDockerDaemonError(err error) bool {
    if err == nil {
        return false
    }
    errStr := strings.ToLower(err.Error())
    return strings.Contains(errStr, "cannot connect to the docker daemon") ||
           strings.Contains(errStr, "docker daemon") ||
           strings.Contains(errStr, "docker.sock") ||
           strings.Contains(errStr, "npipe") || // Windows named pipes
           strings.Contains(errStr, "//./pipe/docker_engine") // Docker Desktop Windows
}
```

## ✅ 7. Configuration Directory Handling

Handle Windows-specific config directory conventions:

```go
// ✅ Good - Windows-aware config paths
func getConfigPath() (string, error) {
    if runtime.GOOS == "windows" {
        // Use %APPDATA% on Windows
        if appdata := os.Getenv("APPDATA"); appdata != "" {
            return filepath.Join(appdata, "container-use"), nil
        }
    }
    // Fall back to Unix-style ~/.config
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, ".config", "container-use"), nil
}
```

## ✅ 8. Terminal and Console Handling

Windows console behavior differs from Unix terminals:

```go
// ✅ Good - Windows console detection
func isWindowsConsole() bool {
    return runtime.GOOS == "windows" && 
           os.Getenv("TERM") == "" // Windows cmd.exe doesn't set TERM
}

// Handle color output appropriately
func setupColorOutput() {
    if isWindowsConsole() {
        // Windows may need special handling for ANSI colors
        // Consider using colorable library for older Windows versions
    }
}
```

## ✅ 9. Build and Deployment

Ensure proper Windows build configuration:

```bash
# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o container-use.exe ./cmd/container-use

# For Windows development
go build -o container-use.exe ./cmd/container-use
```

**Binary naming:**
- Windows executables should have `.exe` extension
- Update install scripts to handle Windows paths correctly

## ✅ 10. Testing on Windows

**Key areas to test:**
- Git repository operations in Windows paths with spaces
- Docker Desktop integration and error handling  
- File path handling with backslashes and long paths
- Console output and color handling
- Signal handling (Ctrl+C behavior)
- PowerShell vs cmd.exe compatibility

**Use Windows-specific test patterns:**
```go
func TestWindowsPathHandling(t *testing.T) {
    if runtime.GOOS != "windows" {
        t.Skip("Windows-only test")
    }
    // Test Windows-specific path scenarios
}
```

## ✅ 11. Dependencies and CGO

Avoid CGO dependencies that may not compile on Windows:

```go
// ✅ Good - Pure Go libraries
"github.com/spf13/cobra"           // CLI framework
"dagger.io/dagger"                 // Container operations
"github.com/dustinkirkland/golang-petname" // Name generation

// ⚠️ Check Windows compatibility before adding new deps
// Some Unix-specific libraries may not work on Windows
```

## ✅ 12. Error Messages and User Experience

Provide Windows-appropriate error messages and help text:

```go
func handleDockerDaemonError() {
    if runtime.GOOS == "windows" {
        fmt.Fprintf(os.Stderr, "\nError: Docker Desktop is not running.\n")
        fmt.Fprintf(os.Stderr, "Please start Docker Desktop and try again.\n\n")
    } else {
        fmt.Fprintf(os.Stderr, "\nError: Docker daemon is not running.\n")
        fmt.Fprintf(os.Stderr, "Please start Docker and try again.\n\n")
    }
}
```

---

**Remember:** container-use is a cross-platform tool that manages containerized environments for AI agents. Windows support should feel native while maintaining the same feature set as Unix platforms.