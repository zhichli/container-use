# Dependency Security Audit

For the dagger/dagger GitHub repository (written in Go).

1. Analyze project dependencies:
   - Setup a sandbox and upload the repository to it.
   - Check the go.mod
   - List all dependencies with versions
   - Identify outdated packages

2. Security check:
   - Check for known vulnerabilities in Go
   - Identify dependencies with critical security issues

3. Upgrade those packages
   - Perform the updates
   - Show a diff
