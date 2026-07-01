# cli-isolate

Per-client LXD VM workspaces with LUKS-encrypted data volumes and cross-client network isolation.

## Features

- **Per-client isolation** - Each client gets a LXD VM, separate LXD project with isolated bridge, and a LUKS-encrypted data volume with its own passphrase
- **EDR-ready** - Client endpoint agents (SentinelOne, CrowdStrike, etc.) run inside the Ubuntu VM on supported OS, not on your host
- **Encrypted at rest** - Data volumes use LUKS2 (AES-256-XTS). Passphrase prompted on `up`, never stored on disk
- **Crash recovery** - `isolate prune` cleans stale LUKS mappings and mounts. `down` is idempotent

## Requirements

- **Linux** with LXD (>= 5.x) and KVM
- `cryptsetup`, `truncate`, `mkfs.btrfs`, `ssh-keygen`
- `rclone` (optional, for `isolate mount`)

## Quickstart

```bash
# Build + install
git clone https://github.com/zinzan-vdm/cli-isolate.git && cd cli-isolate
go build -o isolate . && sudo cp isolate /usr/local/bin/

# Create a workspace (prompts for LUKS passphrase)
isolate create client-a

# Start VM and unlock data volume (prompts for passphrase again)
isolate up client-a

# Open shell in ~/workspace
isolate exec client-a

# Mount workspace on host via rclone SFTP
isolate mount client-a ~/projects/client-a

# Stop VM and lock data volume
isolate down client-a
```

## Commands

| Command | Description |
|---|---|
| `create <name>` | Provision LXD VM + LUKS data volume (100G default, --size, --image, --provision) |
| `up <name>` | Start VM, unlock LUKS inside guest, mount ~/workspace |
| `down <name>` | Close LUKS, unmount host mount, stop VM (idempotent) |
| `exec <name> [cmd]` | Run command inside VM (default: bash in ~/workspace) |
| `mount <name> <path>` | rclone SFTP mount of ~/workspace to host path |
| `umount <name>` | Unmount host-side mount |
| `list` | Table of all isolates: name, VM state, LUKS state, mount path |
| `info <name>` | VM status, IP, SSH command, volume path, size |
| `delete <name>` | Wipe LUKS header, destroy LXD project + VM, remove files |
| `scp push/pull <name> <local> <remote>` | Copy files between host and VM |
| `prune` | Clean stale LUKS mappings, mount state, IP files |

Non-interactive: `create` and `up` support `--password-stdin` to read the LUKS passphrase from stdin. `delete` supports `--force` to skip confirmation.

## State

```
~/.cli-isolate/<name>/
  config.yaml       - image, user, size, LXD project
  data.img          - sparse LUKS-encrypted btrfs volume
  id_ed25519*       - SSH key pair for rclone/scp
  active_ip         - set on `up`, read by `mount`/`scp`
  active_mount      - tracks host mount path, cleaned by `down`
```

## macOS

LXD is Linux-native. Three options:

- **Remote LXD server** - Install `lxc` on macOS via Homebrew, add your VPS as a remote (`lxc remote add`). `isolate mount` uses rclone SFTP over SSH to the server.
- **OrbStack** - `brew install orbstack` gives a fast Linux VM with LXD preconfigured. The `lxc` CLI works natively from macOS.
- **Multipass** - `brew install multipass && multipass launch lxd-vm` with snap LXD inside.

## Testing

```bash
go test -v ./...          # unit tests (config, parsing) + CLI e2e (help, errors)
go test -v ./e2e/ -run 'TestFullLifecycle'   # full lifecycle (needs KVM)
```

## License

MIT. Issues: https://github.com/zinzan-vdm/cli-isolate/issues