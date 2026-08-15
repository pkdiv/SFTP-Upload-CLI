# Uplift

Interactive command-line tool for uploading local files to a remote server over SFTP.

## Features

- Reusable named upload profiles stored in YAML
- Interactive per-file confirmation before upload
- Safe defaults — Enter always means No
- Path traversal protection on local and remote paths
- Symlink safety — no escaping the local base directory
- SSH private key authentication
- Secure host-key verification via `known_hosts`
- No silent overwrites — existing remote files require explicit approval
- Concurrent uploads with `--concurrency`
- Readable output during parallel transfers
- Full profile management (add, list, show, edit, remove)

## Build

```bash
go build -o uplift ./cmd/uplift
```

On Windows use `uplift.exe`.

## Usage

### Create a profile

```bash
uplift profile add
```

Interactive setup:

```text
Profile name: production
Host: example.com
Port [22]:
Username: deploy
Private key: ~/.ssh/id_ed25519
Local base path: /home/user/project
Remote base path: /var/www/app

Save this profile? [y/N]: y
```

### List profiles

```bash
uplift profile list
```

### Show a profile

```bash
uplift profile show production
```

### Edit a profile

```bash
uplift profile edit production
```

### Remove a profile

```bash
uplift profile remove production
```

### Upload files

```bash
uplift upload production src/main.go config/app.yaml
```

With concurrency:

```bash
uplift upload production --concurrency 4 file1.iso file2.iso file3.iso
```

From a file list:

```bash
uplift upload production --files-list files.txt
```

The tool resolves each relative path against the profile's local and remote base paths, then prompts for confirmation before uploading.

## Configuration

The default config location is platform-appropriate:

| Platform | Path |
|---|---|
| Linux | `~/.config/uplift/config.yaml` |
| macOS | `~/Library/Application Support/uplift/config.yaml` |
| Windows | `%AppData%\uplift\config.yaml` |

Override with `--config`:

```bash
uplift --config ./custom.yaml profile list
```

### YAML format

```yaml
profiles:
  production:
    host: example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/id_ed25519
    local_base: /home/user/project
    remote_base: /var/www/app
```

## Security

- Private keys are never stored in YAML or logged
- `known_hosts` host-key verification is required — unknown hosts are rejected
- Path traversal and absolute paths are rejected
- Symlinks resolving outside the local base are rejected
- Existing remote files are never overwritten silently
- Remote directories are only created after the file's upload is approved

## Path mapping

```text
local_base  + relative path = complete local path
remote_base + relative path = complete remote path
```

Example:

```text
Local base:   /home/user/project
Relative:     src/main.go
Local path:   /home/user/project/src/main.go

Remote base:  /var/www/app
Relative:     src/main.go
Remote path:  /var/www/app/src/main.go
```

## Development

Run tests:

```bash
go test ./...
```

Run tests without integration tests:

```bash
go test ./... -short
```

## Project structure

```text
cmd/uplift/          entry point
internal/cli/        commands, flags, exit codes
internal/config/     YAML profiles, validation, CRUD
internal/pathutil/   path resolution, containment, symlink safety
internal/confirmation/  interactive yes/no prompts
internal/sshclient/  SSH auth, host-key verification, SFTP sessions
internal/uploader/   worker pool, remote dir creation, overwrite checks
internal/output/     synchronized terminal output
```
