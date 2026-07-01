# cli-isolate

Manage per-client LXD VM workspaces with LUKS-encrypted data volumes.

Each **isolate** is a disposable LXD VM with a persistent encrypted data volume
mounted at `~/workspace` inside the VM. VMs run in isolated LXD projects with
separate bridge networks for cross-client security.

## Motivation

When contractors require endpoint security software (e.g. SentinelOne) on
personal machines for compliance, `cli-isolate` creates firewalled VMs where:

1. The EDR agent runs inside the VM on a supported OS (Ubuntu)
2. Your actual data lives on a LUKS-encrypted volume attached to the VM
3. Each client gets their own VM, LXD project, bridge network, and passphrase
4. Data is fully encrypted at rest when the isolate is down

## Requirements

- [LXD](https://linuxcontainers.org/lxd/) (>= 5.x) with a properly initialized daemon
- `cryptsetup`, `truncate`, `mkfs.btrfs`, `ssh-keygen`
- `rclone` (optional, for host-side mounts via `isolate mount`)
- Ubuntu host or any distribution with LXD installed

## Install

```bash
go install github.com/zinzan-vdm/cli-isolate@latest
# Or build from source:
git clone https://github.com/zinzan-vdm/cli-isolate.git
cd cli-isolate
go build -o isolate .
sudo cp isolate /usr/local/bin/
```

## Quickstart

```bash
# Create a new workspace
isolate create client-a

# Start the VM and unlock the data volume
isolate up client-a

# Open a shell in the VM (drops into ~/workspace)
isolate exec client-a

# Mount the workspace on your host filesystem
isolate mount client-a ~/projects/client-a

# Stop and lock
isolate down client-a
```

## Commands

| Command | Description |
|---|---|
| `create <name>` | Provision a new LXD VM + encrypted data volume |
| `up <name>` | Start the VM and unlock+mount the data volume |
| `down <name>` | Stop the VM and lock the data volume |
| `exec <name>` | Open a shell inside the VM (cwd: ~/workspace) |
| `mount <name> <path>` | rclone SFTP mount of ~/workspace to host path |
| `umount <name>` | Unmount from the host |
| `list` | Show all isolates with VM/LUKS/mount status |
| `info <name>` | Detailed info about an isolate |
| `delete <name>` | Permanently destroy an isolate and its data |
| `scp push <name> <local> <remote>` | Copy files from host into the isolate |
| `scp pull <name> <remote> <local>` | Copy files from the isolate to host |
| `prune` | Clean up stale LUKS mappings and mount state |

## Architecture

```
~/.cli-isolate/
  client-a/
    config.yaml        ← Metadata (image, user, size, project)
    data.img           ← Sparse LUKS-encrypted Btrfs volume
    id_ed25519         ← SSH key pair for SSHFS/rclone/scp
    id_ed25519.pub
    active_ip          ← Written on `up`, read by `mount`
    active_mount       ← Tracks the host mount path
```

```
┌───────────────────────────────────────────────┐
│ NixOS Host                                    │
│  ~/projects/client-a/ (rclone SFTP mount)     │
│                                                │
│  ┌─────────────────────────────────────────┐  │
│  │ LXD Project: cli-isolate-client-a       │  │
│  │ Bridge: 10.42.x.0/24                    │  │
│  │                                          │  │
│  │ Ubuntu VM                                │  │
│  │  /home/client-a/workspace/         │  │
│  │    ↕ Btrfs mount                         │  │
│  │  /dev/mapper/cr-client-a              │  │
│  │    ↕ LUKS                                │  │
│  │  /dev/vdb (attached data.img)            │  │
│  │                                          │  │
│  │  Also running: SentinelOne agent         │  │
│  └─────────────────────────────────────────┘  │
└───────────────────────────────────────────────┘
```
