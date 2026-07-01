# cli-isolate

**Per-client LXD VM workspaces with LUKS-encrypted data volumes.**

Each **isolate** is a disposable LXD VM with a persistent encrypted data volume
mounted at `~/workspace` inside the VM. VMs run in isolated LXD projects with
separate bridge networks — no cross-client network access.

## Motivation

When clients require endpoint security software (e.g. SentinelOne,
CrowdStrike, Defender for Endpoint) on your personal machine for compliance,
`cli-isolate` creates firewalled VMs where:

1. The EDR agent runs inside the VM on a supported OS (Ubuntu LTS)
2. Your actual data lives on a LUKS-encrypted volume attached to the VM
3. Each client gets their own VM, LXD project, bridge network, and passphrase
4. Data is fully encrypted at rest when the isolate is down

### Cross-client isolation

The data volumes are **never shared** between clients:

- Each `.img` is a separate LUKS volume with its own passphrase
- Each VM runs in its own [LXD project](https://linuxcontainers.org/lxd/docs/latest/projects/)
  with a dedicated bridge network (`10.x.0.0/24`, NAT to internet)
- Client A's VM cannot reach client B's VM at the network level
- When working on client B, client A's volume is locked and its VM is stopped
- The `up` command warns if another isolate is already running and requires
  confirmation before allowing concurrent access

## Requirements

- **Linux** with [LXD](https://linuxcontainers.org/lxd/) (>= 5.x) and KVM
- `cryptsetup`, `truncate`, `mkfs.btrfs`, `ssh-keygen`
- `rclone` (optional, for `isolate mount`)

```bash
# Ubuntu / Debian
sudo apt install lxd cryptsetup btrfs-progs rclone

# NixOS
{ pkgs, ... }: {
  environment.systemPackages = with pkgs; [ lxd cryptsetup btrfs-progs rclone ];
  virtualisation.lxd.enable = true;
}
```

## Install

```bash
git clone https://github.com/zinzan-vdm/cli-isolate.git
cd cli-isolate
go build -o isolate .
sudo cp isolate /usr/local/bin/
```

Or with Go installed:

```bash
go install github.com/zinzan-vdm/cli-isolate@latest
```

## Quickstart

```bash
# 1. Create a workspace for client-a
#    Prompts for a LUKS passphrase (twice for confirmation).
#    Provisions: LUKS volume, SSH key, LXD project + network, Ubuntu VM.
isolate create client-a

# 2. Start the VM and unlock the data volume
#    Prompts for the LUKS passphrase again (decrypt inside the VM).
isolate up client-a

# 3. Open a shell in the VM (drops into ~/workspace)
isolate exec client-a

# 4. Mount the workspace on your host filesystem (uses rclone SFTP)
isolate mount client-a ~/projects/client-a

# 5. When done: unmount host, lock LUKS, stop VM
isolate down client-a
```

## Commands

### Lifecycle

| Command | Description |
|---|---|
| `create <name> [flags]` | Provision a new LXD VM + LUKS data volume |
| `up <name>` | Start VM, prompt LUKS passphrase, decrypt inside VM, mount `~/workspace` |
| `down <name>` | Close LUKS, unmount host mount, stop VM (idempotent) |
| `delete <name>` | Wipe LUKS header, destroy LXD project + VM, remove files |

### Interaction

| Command | Description |
|---|---|
| `exec <name> [cmd...]` | Run command inside VM (default: bash in `~/workspace`) |
| `mount <name> <host-path>` | rclone SFTP mount of `~/workspace` to a host directory |
| `umount <name>` | Unmount the host-side mount |
| `scp push <name> <local> <remote>` | Copy files from host into the VM |
| `scp pull <name> <remote> <local>` | Copy files from the VM to host |

### Management

| Command | Description |
|---|---|
| `list` | Table: name, VM state, LUKS state, mount path |
| `info <name>` | Full details: VM status, IP, SSH command, volume path, size |
| `prune` | Clean stale LUKS mappings, mount state, IP files (crash recovery) |

### Flags

| Flag | Commands | Default | Description |
|---|---|---|---|
| `--image` / `-i` | `create` | `ubuntu:24.04` | LXD image for the VM |
| `--size` / `-s` | `create` | `100G` | Data volume size (e.g. `50G`, `200G`) |
| `--provision` / `-p` | `create` | — | Optional script to run inside the VM after setup |
| `--password-stdin` | `create`, `up` | — | Read LUKS passphrase from stdin (non-interactive) |
| `--force` / `-f` | `delete` | — | Skip confirmation prompts |

## Architecture

### File layout

```
~/.cli-isolate/
  client-a/
    config.yaml        ← Metadata (image, user, size, project)
    data.img           ← Sparse LUKS-encrypted Btrfs volume (100G)
    id_ed25519         ← SSH key pair for rclone/scp
    id_ed25519.pub
    active_ip          ← Written on `up`, read by `mount` and `scp`
    active_mount       ← Tracks the host mount path
```

### Network isolation

```
┌──────────────────────────────────────────────────┐
│ Host                                              │
│  ~/projects/client-a/ (rclone SFTP mount)         │
│                                                    │
│  ┌─ LXD Project: cli-isolate-client-a ──────────┐ │
│  │  Bridge: br-4f958e81 (10.72.0.0/24)          │ │
│  │  No route to other projects                   │ │
│  │                                                │ │
│  │  ┌─ Ubuntu VM (4 vCPU, 8 GB) ──────────────┐  │ │
│  │  │  /home/client-a/workspace/             │  │ │
│  │  │    ↕ btrfs mount                         │  │ │
│  │  │  /dev/mapper/cr-client-a              │  │ │
│  │  │    ↕ LUKS (passphrase, key in memory)    │  │ │
│  │  │  /dev/vdb (attached data.img)            │  │ │
│  │  │                                           │  │ │
│  │  │  Also running: EDR agent (SentinelOne)   │  │ │
│  │  └───────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### Security model

| Layer | Protection |
|---|---|
| **LUKS** | Data encrypted at rest (AES-256-XTS). Passphrase prompted on `up`, never stored. |
| **LXD project** | Each VM in isolated project with its own bridge. No cross-VM networking. |
| **Separate passphrases** | Each client's volume has a unique passphrase — compromise of one doesn't affect others. |
| **No host-side decryption** | LUKS is opened inside the VM. The host only sees encrypted `.img` files. |

### Crash recovery

`isolate prune` detects and cleans orphaned state from crashes:

- Stale LUKS mappings on the host (if `isolate up` was interrupted)
- Stale rclone mounts (if the host crashed while a mount was active)
- Orphaned state files (`active_ip`, `active_mount`)

`isolate down` is fully idempotent — safe to run if a previous `down` was
interrupted.

## macOS

`cli-isolate` requires LXD which is Linux-native. Three paths for macOS:

### A) Remote LXD server (recommended for your VPS setup)

Run LXD on a Linux server (e.g. your Hetzner VPS). Install the `lxc` client
on macOS and add the server as a remote:

```bash
brew install lxc
lxc remote add my-server https://<vps-ip>:8443 --accept-cert
lxc remote switch my-server
# Now isolate create/up/down/exec all work via the remote
# isolate mount uses rclone SFTP over SSH to the server
```

### B) OrbStack (local VM)

[OrbStack](https://orbstack.dev/) provides a fast Linux VM with LXD:

```bash
brew install orbstack
# LXD comes pre-configured; lxc CLI works natively from macOS
```

### C) Multipass

```bash
brew install multipass
multipass launch --name lxd-vm --cpus 4 --mem 8G
multipass exec lxd-vm -- sudo snap install lxd
# Forward LXD to macOS and use as a remote (same as option A)
```

## Testing

```bash
# Unit tests (logic, config, parsing)
go test -v ./

# CLI e2e tests (help, errors, validation — no LXD needed)
go test -v ./e2e/

# Full lifecycle e2e (requires KVM + LXD)
go test -v ./e2e/ -run 'TestFullLifecycle'

# All tests
go test -v ./...
```

## Developing

```bash
git clone https://github.com/zinzan-vdm/cli-isolate.git
cd cli-isolate
go build -o isolate .
./isolate --help
```

The tool is a single Go binary with no runtime dependencies beyond
`lxc`, `cryptsetup`, `mkfs.btrfs`, `ssh-keygen`, and optionally `rclone`.