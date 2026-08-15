# AGENTS.md

# SFTP Upload CLI

## 1. Project Overview

Build an interactive CLI tool written in **Go** for uploading selected local files to a remote server over **SFTP**.

The tool accepts a list of files relative to a configured local base path and uploads them to the corresponding location relative to a configured remote base path.

The tool must support reusable named upload configurations stored in YAML.

A configuration contains:

* SSH server details
* SSH authentication details
* Local base path
* Remote base path
* A user-defined configuration name

This allows subsequent uploads to reuse the same configuration without requiring the user to specify the local and remote base paths again.

---

# 2. Core Design

The fundamental mapping is:

```text
configured local base
        +
relative file path
        ↓
complete local path

configured remote base
        +
relative file path
        ↓
complete remote path
```

Example configuration:

```yaml
profiles:
  production:
    host: example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/id_ed25519
    local_base: /home/user/project
    remote_base: /var/www/app

  staging:
    host: staging.example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/staging_ed25519
    local_base: /home/user/project
    remote_base: /var/www/staging
```

Then the user can run:

```bash
sftp-upload upload production src/main.go config/app.yaml
```

The tool resolves:

```text
/home/user/project/src/main.go
    →
/var/www/app/src/main.go
```

and:

```text
/home/user/project/config/app.yaml
    →
/var/www/app/config/app.yaml
```

---

# 3. Technology

The application must be implemented in:

* **Go**
* YAML for configuration
* SSH/SFTP libraries appropriate for Go
* Standard Go libraries wherever practical

Do not use:

* Java
* Rust

Keep the application as a single CLI executable.

---

# 4. Interactive Requirement

The application is fundamentally an **interactive CLI**.

The user must be able to understand what is about to happen before any file is uploaded.

Every individual file upload requires explicit confirmation.

For every file, display:

```text
Local:
  /home/user/project/src/main.go

Remote:
  /var/www/app/src/main.go

Upload this file? [y/N]:
```

Only an explicit `y` or `Y` should authorize the upload.

The default must always be **No**.

Pressing Enter must not upload the file.

Invalid input should cause the prompt to be repeated.

---

# 5. Configuration Profiles

The YAML configuration must store complete reusable upload profiles.

A profile represents a reusable combination of:

```text
SSH server
SSH authentication
local base path
remote base path
```

Each profile has a unique user-defined name.

Example:

```yaml
profiles:
  production:
    host: example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/id_ed25519
    local_base: /home/user/project
    remote_base: /var/www/app

  staging:
    host: staging.example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/staging_ed25519
    local_base: /home/user/project
    remote_base: /var/www/staging
```

The profile name is used by the upload command.

Example:

```bash
sftp-upload upload production src/main.go
```

---

# 6. Configuration Management

The CLI should provide interactive configuration management.

Recommended commands:

```bash
sftp-upload profile add
sftp-upload profile list
sftp-upload profile show <name>
sftp-upload profile edit <name>
sftp-upload profile remove <name>
```

The exact command names may be adjusted during implementation, but the functionality must exist.

---

## 6.1 Creating a Profile

Creating a profile must be interactive.

Example:

```text
Create SFTP upload profile

Profile name: production
Host: example.com
Port [22]: 22
Username: deploy
Private key: ~/.ssh/id_ed25519

Local base path:
  /home/user/project

Remote base path:
  /var/www/app
```

Before saving, display the resulting configuration:

```text
Profile:
  production

Server:
  example.com:22

Username:
  deploy

Private key:
  ~/.ssh/id_ed25519

Local base:
  /home/user/project

Remote base:
  /var/www/app

Save this profile? [y/N]:
```

Only save after explicit confirmation.

Never display private key contents.

---

# 7. Upload Command

The primary operation should be conceptually:

```bash
sftp-upload upload <profile> <files...>
```

Example:

```bash
sftp-upload upload production \
  src/main.go \
  src/config.go \
  config/app.yaml
```

The profile supplies:

```text
local base path
remote base path
server
SSH credentials
```

The command only needs to provide the relative file paths.

---

# 8. Optional Base Path Overrides

The MVP should prefer the stored profile configuration.

If explicit command-line overrides are implemented, they must be clearly distinguishable from the stored profile.

Potential future syntax:

```bash
sftp-upload upload production \
  --local-base /tmp/build \
  --remote-base /var/www/app \
  dist/app.js
```

Overrides should never silently modify the stored profile.

They only affect the current invocation unless the user explicitly chooses to update the profile.

---

# 9. Path Mapping

For each supplied relative file:

```text
local_path  = local_base + relative_path
remote_path = remote_base + relative_path
```

Use proper path manipulation utilities.

Do not construct paths using naive string concatenation.

Example:

```text
Local base:
  /home/user/project

Relative:
  src/main.go

Result:
  /home/user/project/src/main.go
```

Remote:

```text
Remote base:
  /var/www/app

Relative:
  src/main.go

Result:
  /var/www/app/src/main.go
```

---

# 10. Path Traversal Protection

This is a security-critical requirement.

Relative paths must remain inside the configured local base directory.

Reject:

```text
../secret.txt
../../etc/passwd
../../../private/key
```

The same principle must apply when constructing remote paths.

For example:

```text
Remote base:
  /var/www/app

Relative path:
  ../../etc/passwd
```

must never result in:

```text
/etc/passwd
```

Use robust path normalization and containment checks.

---

# 11. Symlink Handling

Symlinks require explicit handling.

A symlink inside the local base must not be allowed to bypass the local base containment check.

Example:

```text
/home/user/project/secret -> /home/user/private
```

Uploading:

```text
secret/password.txt
```

must not accidentally allow:

```text
/home/user/private/password.txt
```

unless symlink behavior has explicitly been designed and approved.

The safe default is to reject files that resolve outside the configured local base.

---

# 12. SSH Configuration

Each profile must contain:

```yaml
host: example.com
port: 22
username: deploy
key_file: ~/.ssh/id_ed25519
```

Port defaults to:

```text
22
```

if omitted.

---

# 13. SSH Authentication

The initial implementation must support:

### Private key authentication

Example:

```yaml
key_file: ~/.ssh/id_ed25519
```

The implementation must:

* Expand `~` where appropriate.
* Verify that the key exists.
* Parse the private key.
* Return useful errors for invalid keys.
* Never print private key contents.
* Support passphrase-protected keys where practical.

Do not store plaintext passwords in YAML.

Password authentication is not required for the MVP.

SSH agent authentication may be added later.

---

# 14. SSH Host Verification

SSH host-key verification must be implemented securely.

Do **not** use:

```go
ssh.InsecureIgnoreHostKey()
```

as the production host-key verification mechanism.

Prefer the user's existing known-hosts configuration where practical.

Never silently accept an unknown or changed host key.

If the application needs an interactive first-connection flow, clearly tell the user what host key is being accepted and require explicit confirmation.

---

# 15. Concurrency / Upload Throttling

The application must support throttling the number of simultaneous uploads.

This must be configurable through an appropriate CLI flag.

Example:

```bash
sftp-upload upload production \
  --concurrency 4 \
  file1.iso \
  file2.iso \
  file3.iso \
  file4.iso
```

The exact flag name may be changed during implementation, but it must clearly represent the maximum number of simultaneous uploads.

Recommended default:

```text
--concurrency 1
```

This preserves simple, predictable behavior by default.

Example:

```text
--concurrency 1
```

means:

```text
one upload at a time
```

while:

```text
--concurrency 4
```

means:

```text
at most four files are being transferred simultaneously
```

---

# 16. Concurrency and Confirmation

Concurrency must **not** weaken the confirmation requirement.

Every file still requires explicit confirmation before its upload begins.

The tool must not automatically approve a group of files merely because concurrency is enabled.

A suitable interactive flow is:

```text
File 1/5

Local:
  /home/user/project/a.bin

Remote:
  /var/www/app/a.bin

Upload this file? [y/N]: y

File 2/5

Local:
  /home/user/project/b.bin

Remote:
  /var/www/app/b.bin

Upload this file? [y/N]: y
```

Once files have been individually approved, the approved files may be scheduled for concurrent upload.

---

# 17. Recommended Upload Model

Separate the workflow into two phases:

## Phase 1 — Interactive approval

Resolve and validate all files.

For each file:

```text
local path
remote path
```

Display the paths and ask for confirmation.

Create an approved upload queue containing only explicitly approved files.

## Phase 2 — Transfer

Upload approved files using the configured concurrency limit.

Conceptually:

```text
Input files
     |
     v
Validate paths
     |
     v
Interactive confirmation
     |
     +---- rejected ---> skipped
     |
     v
Approved files
     |
     v
Concurrency-limited workers
     |
     v
SFTP uploads
```

This separation makes the security boundary obvious.

---

# 18. Why Approval Should Precede Concurrent Uploads

Do not ask for confirmations from multiple concurrent worker goroutines.

Avoid output such as:

```text
File A? [y/N]:
File B? [y/N]:
File C? [y/N]:
```

appearing simultaneously.

Interactive input must remain serialized and deterministic.

All user confirmations should happen in the main interactive phase.

Only after approval should concurrent workers start.

---

# 19. Existing Remote Files

The tool must not silently overwrite an existing remote file.

Before uploading an existing remote file, display:

```text
Remote file already exists:

  /var/www/app/config/app.yaml

Overwrite? [y/N]:
```

The user must explicitly approve the overwrite.

The general upload confirmation and overwrite confirmation may be combined if the resulting UX remains completely unambiguous.

Never overwrite by default.

---

# 20. Remote Directory Creation

If the destination directory does not exist, the application should handle it explicitly.

Recommended behavior:

```text
Remote directory does not exist:

  /var/www/app/config

Create it? [y/N]:
```

The directory must only be created after explicit approval.

Alternatively, the application may create missing directories automatically after the file's upload confirmation if this behavior is clearly documented and predictable.

Do not create directories outside the intended remote base.

---

# 21. Local File Validation

Before presenting a file for confirmation, verify that:

* It exists.
* It is a regular file.
* It is readable.
* It remains inside the configured local base.
* Symlink resolution does not escape the local base.

Directories are not accepted as files in the MVP.

Broken symlinks must be rejected.

---

# 22. CLI Flags

The CLI should expose appropriate flags without unnecessarily duplicating profile configuration.

At minimum, the upload command should support:

```text
--concurrency <N>
```

Example:

```bash
sftp-upload upload production --concurrency 4 file1 file2 file3
```

Useful future flags include:

```text
--dry-run
--verbose
--quiet
```

Do not implement flags that bypass confirmation unless explicitly requested.

In particular, do not introduce `--yes` or `--force` in the MVP.

---

# 23. Interactive CLI Behavior

The CLI must remain interactive even when multiple files are provided.

Example:

```text
SFTP Upload
────────────────────────────────────

Profile:
  production

Server:
  deploy@example.com:22

Local base:
  /home/user/project

Remote base:
  /var/www/app

Concurrency:
  3

Files:
  4

────────────────────────────────────

File 1/4

Local:
  /home/user/project/src/main.go

Remote:
  /var/www/app/src/main.go

Upload this file? [y/N]:
```

The application should make the active profile and path mapping visible before processing files.

---

# 24. Upload Progress

When concurrency is greater than one, output must remain readable.

Avoid multiple goroutines writing uncontrolled output directly to stdout.

Use a centralized output mechanism.

Possible output:

```text
[1/4] Uploading src/main.go
[2/4] Uploading config/app.yaml
[1/4] ✓ Uploaded
[3/4] Uploading assets/logo.png
[2/4] ✓ Uploaded
```

The exact UI can evolve.

The important requirement is that concurrent uploads do not make the terminal unusable.

---

# 25. Final Summary

After all uploads finish, display a summary.

Example:

```text
Upload complete
────────────────────────────────────

Uploaded:  3
Skipped:   1
Failed:    0

Profile:
  production

Concurrency:
  3
```

If failures occur:

```text
Upload complete
────────────────────────────────────

Uploaded:  2
Skipped:   1
Failed:    1

Failed files:

  /home/user/project/a.txt
    → /var/www/app/a.txt
    permission denied
```

---

# 26. Configuration File Location

Use platform-appropriate per-user configuration directories.

Examples:

```text
Linux:
~/.config/<application>/config.yaml

macOS:
~/Library/Application Support/<application>/config.yaml

Windows:
%AppData%\<application>\config.yaml
```

Support an explicit configuration file override such as:

```bash
--config ./config.yaml
```

The exact application name may be decided during implementation.

---

# 27. Suggested YAML Schema

The initial schema should resemble:

```yaml
profiles:
  production:
    host: example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/id_ed25519

    local_base: /home/user/project
    remote_base: /var/www/app

  staging:
    host: staging.example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/staging_ed25519

    local_base: /home/user/project
    remote_base: /var/www/staging
```

Do not store:

```yaml
password: ...
private_key: ...
```

in the configuration.

The private key should remain in its own file.

---

# 28. Profile Name Requirements

Profile names must:

* Be unique.
* Be required.
* Be easy to type.
* Be usable directly from the CLI.
* Not contain path separators.

Examples:

```text
production
staging
development
customer-a
backup-server
```

If a profile already exists, the CLI must not silently overwrite it.

Require explicit confirmation before replacing it.

---

# 29. Configuration Editing

`profile edit` should be interactive.

Example:

```text
Edit profile: production

Host [example.com]:
Port [22]:
Username [deploy]:
Private key [~/.ssh/id_ed25519]:

Local base [/home/user/project]:
Remote base [/var/www/app]:
```

Pressing Enter should retain the existing value.

Before saving:

```text
Save changes to profile 'production'? [y/N]:
```

---

# 30. Configuration Listing

Example:

```bash
sftp-upload profile list
```

Output:

```text
NAME          HOST                    LOCAL BASE
production    example.com:22          /home/user/project
staging       staging.example.com:22  /home/user/project
backup        backup.example.com:22   /home/user/backups
```

Do not display private key contents.

---

# 31. Profile Display

Example:

```bash
sftp-upload profile show production
```

Output:

```text
Profile: production

Host:
  example.com:22

Username:
  deploy

Private key:
  ~/.ssh/id_ed25519

Local base:
  /home/user/project

Remote base:
  /var/www/app
```

---

# 32. Architecture

Keep the application modular.

Suggested structure:

```text
cmd/
    sftp-upload/

internal/
    cli/
    config/
    confirmation/
    pathutil/
    sshclient/
    uploader/
    output/
```

### `config`

Responsible for:

* YAML parsing
* Configuration validation
* Loading profiles
* Saving profiles
* Profile CRUD operations

### `confirmation`

Responsible for:

* Interactive prompts
* Yes/no parsing
* Safe defaults

### `pathutil`

Responsible for:

* Local path resolution
* Remote path resolution
* Path normalization
* Containment validation
* Symlink safety

### `sshclient`

Responsible for:

* SSH connection
* Host-key verification
* Authentication
* SFTP session creation

### `uploader`

Responsible for:

* Uploading files
* Remote directory handling
* Existing-file detection
* Concurrency management

### `output`

Responsible for:

* Consistent terminal output
* Progress reporting
* Concurrent upload status
* Final summaries

### `cli`

Responsible for:

* Argument parsing
* Commands
* Flags
* Exit codes

Keep business logic outside the CLI package.

---

# 33. Concurrency Architecture

Use a bounded worker model.

Conceptually:

```text
Approved files
      |
      v
   Job Queue
      |
      +---- Worker 1
      |
      +---- Worker 2
      |
      +---- Worker 3
      |
      +---- Worker N
```

Where:

```text
N = --concurrency
```

The concurrency value must be validated.

Reject:

```text
--concurrency 0
--concurrency -1
```

A reasonable upper bound may also be enforced to prevent accidental resource exhaustion.

The default should be:

```text
1
```

---

# 34. SFTP Connection Strategy

The implementation should explicitly decide whether workers:

1. Share one SFTP connection,
2. Share an SSH connection but create separate SFTP sessions, or
3. Maintain a bounded pool of SFTP connections.

The chosen implementation must respect the concurrency capabilities and thread-safety guarantees of the selected SFTP library.

Do not assume an SFTP client is safe for concurrent use without verifying the library's behavior.

A bounded connection/session pool is preferred if the library requires separate sessions for concurrent transfers.

---

# 35. Security Requirements

Never:

* Log private keys.
* Log passwords.
* Store passwords in plaintext YAML.
* Disable SSH host-key verification.
* Upload files without explicit confirmation.
* Silently overwrite remote files.
* Allow local path traversal.
* Allow remote path traversal.
* Follow symlinks outside the configured local base.
* Create arbitrary remote directories.

Security-sensitive operations must have tests.

---

# 36. Error Handling

Errors should identify:

* Operation
* Profile
* File when applicable
* Cause

Example:

```text
Failed to upload:

  Local:
    /home/user/project/config/app.yaml

  Remote:
    /var/www/app/config/app.yaml

Reason:
  permission denied
```

Never include sensitive credential material.

---

# 37. Exit Codes

Minimum behavior:

```text
0
```

when all requested operations completed successfully or were intentionally skipped.

Non-zero when one or more requested uploads failed.

If some files succeed and others fail, return a non-zero exit code.

User-declined uploads are not failures.

---

# 38. Testing Requirements

Tests are mandatory for:

### Configuration

* Valid YAML
* Invalid YAML
* Missing profile
* Duplicate profile
* Missing host
* Missing username
* Missing key
* Invalid port
* Profile persistence
* Local base persistence
* Remote base persistence

### Path handling

* Normal paths
* Nested paths
* `./`
* `../`
* `../../`
* Absolute paths
* Symlinks
* Symlinks escaping the base
* Windows path handling where applicable

### Confirmation

* `y`
* `Y`
* `n`
* `N`
* Enter
* Invalid input

### Upload

* Successful upload
* Authentication failure
* Connection failure
* Missing local file
* Permission failure
* Existing remote file
* Missing remote directory
* Multiple simultaneous uploads
* Concurrency limit enforcement

---

# 39. Integration Testing

Use a disposable SSH/SFTP server for end-to-end tests.

Do not rely exclusively on mocks.

Integration tests should verify:

```text
local file
    ↓
CLI
    ↓
profile
    ↓
SSH authentication
    ↓
SFTP
    ↓
correct remote path
    ↓
correct file contents
```

Also test concurrent uploads with different concurrency values.

---

# 40. MVP Requirements

The MVP is complete when all of the following work:

* [ ] Go CLI application exists.
* [ ] Interactive operation is implemented.
* [ ] YAML configuration exists.
* [ ] Named profiles exist.
* [ ] Profiles contain SSH configuration.
* [ ] Profiles contain local base path.
* [ ] Profiles contain remote base path.
* [ ] Profiles can be created interactively.
* [ ] Profiles can be listed.
* [ ] Profiles can be inspected.
* [ ] Profiles can be edited.
* [ ] Profiles can be removed.
* [ ] Upload command accepts a profile name.
* [ ] Upload command accepts multiple relative file paths.
* [ ] Complete local paths are displayed.
* [ ] Complete remote paths are displayed.
* [ ] Every file requires explicit confirmation.
* [ ] Enter defaults to No.
* [ ] Path traversal is prevented.
* [ ] Unsafe symlinks are rejected.
* [ ] SSH private-key authentication works.
* [ ] SSH host-key verification is secure.
* [ ] Existing remote files are never silently overwritten.
* [ ] Missing remote directories are handled safely.
* [ ] Concurrent uploads are supported.
* [ ] `--concurrency` controls simultaneous uploads.
* [ ] Default concurrency is 1.
* [ ] Concurrent terminal output remains readable.
* [ ] Upload results are summarized.
* [ ] Failed uploads produce a non-zero exit code.
* [ ] Unit tests exist for security-critical functionality.
* [ ] SFTP integration tests exist.

---

# 41. Example End-to-End Usage

Create a profile:

```bash
sftp-upload profile add
```

Interactive setup:

```text
Profile name: production
Host: example.com
Port [22]: 22
Username: deploy
Private key: ~/.ssh/id_ed25519

Local base:
  /home/user/my-app

Remote base:
  /var/www/my-app

Save profile? [y/N]: y

✓ Profile 'production' saved.
```

Later, upload files without reconfiguring paths:

```bash
sftp-upload upload production \
  --concurrency 3 \
  build/app.js \
  build/app.css \
  config/production.yaml
```

The tool displays:

```text
Profile: production
Local base:  /home/user/my-app
Remote base: /var/www/my-app
Concurrency: 3

────────────────────────────────────

Local:
  /home/user/my-app/build/app.js

Remote:
  /var/www/my-app/build/app.js

Upload this file? [y/N]:
```

The same happens for every file.

Only explicitly approved files enter the concurrent upload queue.

---

# 42. Important Design Rule

The application has two distinct concepts:

### Profile

Persistent configuration:

```text
Who/where am I connecting to?
What local tree am I uploading from?
What remote tree am I uploading to?
```

### Upload invocation

Temporary operation:

```text
Which files am I uploading now?
How many uploads may run concurrently?
```

Do not mix these concepts.

For example:

```bash
sftp-upload upload production --concurrency 4 src/a src/b src/c
```

means:

```text
production
    ↓
load stored SSH configuration
    ↓
load stored local base
    ↓
load stored remote base
    ↓
resolve src/a, src/b, src/c
    ↓
ask for confirmation for each
    ↓
upload approved files with max 4 concurrent transfers
```

This separation should remain central to the architecture.

---

# 43. Non-Negotiable Safety Rule

**No file may be uploaded unless the user has explicitly approved that specific file during the current interactive invocation.**

Concurrency must never bypass this requirement.

The user must always have the opportunity to see:

```text
COMPLETE LOCAL PATH
        ↓
COMPLETE REMOTE PATH
```

before that file is transferred.

The reusable YAML profile removes repetitive configuration; it must never remove the per-upload confirmation requirement.
