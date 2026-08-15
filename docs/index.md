# Uplift

Interactive command-line tool for uploading local files to a remote server over SFTP.

## Features

- Reusable named upload profiles stored in YAML
- Interactive TUI — no long commands needed
- Per-file upload confirmation with full path mapping
- Safe defaults — Enter always means No
- Path traversal protection on local and remote paths
- Symlink safety — no escaping the local base directory
- SSH private key authentication
- Secure host-key verification via `known_hosts`
- No silent overwrites — existing remote files require explicit approval
- Batched upload confirmation (5 files at a time, toggleable)
- Concurrent uploads with `--concurrency`
- Session logs with timestamps and upload status
- Full profile management (add, list, show, edit, remove)

## Build

```bash
go build -o uplift ./cmd/uplift
```

On Windows use `uplift.exe`.

## Interactive Mode

Run `uplift` with no arguments to launch the TUI.

```bash
uplift
```

### TUI Flags

```bash
uplift --home
```

Opens the file picker in the user's home directory instead of the profile's local base.

```bash
uplift --files-list files.txt
```

Reads relative file paths from a text file (one per line, `#` for comments) and skips manual file selection, jumping directly to upload confirmation.

```bash
uplift --home --files-list files.txt
```

Both flags can be combined.

### Screens

**Profile picker** — arrow keys to navigate, `enter` to select, `a` to add, `e` to edit, `d` to delete, `q` to quit.

**File picker** — `space` to toggle selection, `enter` to proceed, `b` to toggle batch mode, `esc` to go back, `q` to quit.

**Confirm upload** — shows local → remote paths with size and modification date for both sides. `y` to confirm, `n` to cancel.

**Summary** — final counts and session log path.

## CLI Commands

### Create a profile

```bash
uplift profile add
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

## Session Logs

Each upload session creates a JSONL log file in `~/.uplift/logs/`.

```json
{"timestamp":"2026-08-15 15:30:21","relative_path":"Resume.pdf","local_path":"C:\\...","remote_path":"/home/dpk/...","local_size":165273,"status":"uploaded"}
```

## Security

- Private keys are never stored in YAML or logged
- `known_hosts` host-key verification is required
- Path traversal and absolute paths are rejected
- Symlinks resolving outside the local base are rejected
- Existing remote files are never overwritten silently
- Remote directories are only created after the file's upload is approved

## Path Mapping

```text
local_base  + relative path = complete local path
remote_base + relative path = complete remote path
```

## Download

Prebuilt binaries are published as GitHub Releases. See the [Releases](https://github.com/pkdiv/uplift/releases) page.
