# Sweep User Guide

## Overview

Sweep scans directories for large files and provides tools to manage disk space. It operates in two modes: an interactive terminal UI (TUI) and a non-interactive command-line mode.

## Interactive TUI Mode

Launch the TUI by running `sweep` without the `-n` flag:

```bash
sweep ~/Downloads
```

### Interface Layout

Both list and tree views share a consistent header structure:

```
 [broom] SWEEP  47 files  1.2 GB  [check] Freed 234 MB  [bullet] LIVE
  Scanned: 1,234 dirs, 5,678 files  |  Time: 2.3s
------------------------------------------------------------
  [Space] Toggle  [a] All  [n] None  [Enter] Delete  [q] Quit
------------------------------------------------------------
```

**Header elements:**
- App icon and title
- File count and total size of large files found
- "Freed X" indicator showing space reclaimed in current session
- "LIVE" indicator when daemon file watching is active
- Scan metrics showing directories/files scanned and elapsed time
- Key hints bar with available actions
- Column headers

### List View (Default)

The list view displays files in a flat list sorted by size (largest first).

```
       Size  File
------------------------------------------------------------
 [check]   1.2 GB  ubuntu-24.04.iso
 [circle] 856.3 MB  node_modules.zip
 [circle] 423.1 MB  backup.tar.gz
```

**Selection indicators:**
- `[check]` (green checkmark): Selected for deletion
- `[circle]` (gray circle): Not selected

**Detail panel:**
When a file is highlighted, additional details appear below the list:
- Full file path
- Last modified date
- File type (extension)
- Owner (if available)

**List view keys:**

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | Move cursor up/down |
| `Space` | Toggle selection on current file |
| `a` | Select all files |
| `n` | Deselect all files |
| `Enter` | Open delete confirmation dialog |
| `g` / `Home` | Jump to first file |
| `G` / `End` | Jump to last file |
| `PgUp` / `PgDn` | Page up/down |
| `t` | Switch to tree view |
| `L` | Toggle log viewer panel |
| `q` / `Esc` | Quit |

### Tree View

The tree view displays files organized by directory hierarchy. Switch to it by pressing `t`.

```
     Name                                        %    Size
------------------------------------------------------------
[down-filled] node_modules                      47% (47 files, 1.2 GB)
  [circle] package-lock.json                     6% 156 MB
[right-outline] Library                         32% (12 files, 823 MB)
[filled-circle] ubuntu-24.04.iso                15% 412 MB
```

**Directory indicators:**
- `[down-filled]` Expanded directory, selected
- `[down-outline]` Expanded directory, not selected
- `[right-filled]` Collapsed directory, selected
- `[right-outline]` Collapsed directory, not selected

**File indicators:**
- `[filled-circle]` File selected
- `[empty-circle]` File not selected

**Percentage column:**
Each row displays its size as a percentage of the total large file bytes. This helps identify which directories contribute most to disk usage.

**Tree view keys:**

| Key | Action |
|-----|--------|
| `j` / `k` / arrows | Move cursor up/down |
| `Enter` | Expand/collapse directory |
| `Space` | Toggle selection (files and directories) |
| `d` | Delete selected items |
| `c` | Clear all selections |
| `t` | Switch to list view |
| `L` | Toggle log viewer panel |
| `q` / `Esc` | Quit |

**Directory selection:**
Selecting a directory marks it for deletion. The staging area shows the count and total size of all large files underneath selected directories.

### Staging Area

When files are selected, a staging area appears showing:

```
  3 selected  -  1.8 GB                   [d]elete  [c]lear
```

### Deletion Workflow

1. Navigate and select files using `Space`
2. Press `Enter` (list) or `d` (tree) to open confirmation dialog
3. Use arrow keys or `Tab` to choose Cancel or Delete
4. Press `Enter` to confirm, or `y` as shortcut for delete

Files are moved to the system trash, not permanently deleted.

After deletion:
- "Freed X" indicator updates in the header
- Files disappear from the list
- Tree view updates parent directory aggregates

### Real-Time Updates

When the daemon is running and watching the scanned path:
- The "LIVE" indicator appears in the header
- New large files appear automatically
- Deleted files (from Finder or other tools) disappear
- Modified files update their size

Notifications appear briefly when files change:
- `[diamond]` New file added
- `[x]` File removed
- `[hollow-diamond]` File modified
- `[arrow]` File renamed

### Log Viewer

Press `L` to toggle the log viewer panel. This shows internal log messages useful for debugging.

**Log viewer keys:**

| Key | Action |
|-----|--------|
| `1` | Show debug level and above |
| `2` | Show info level and above |
| `3` | Show warn level and above |
| `4` | Show error level only |
| `j` / `k` | Scroll log entries |
| `L` or `Esc` | Close log viewer |

## Non-Interactive Mode

Add `-n` or specify an output format to run without the TUI:

```bash
sweep -n ~/Downloads
sweep -o json ~/Downloads
```

### Output Formats

| Format | Flag | Description |
|--------|------|-------------|
| pretty | `-o pretty` | Colored table (default for `-n`) |
| plain | `-o plain` | Plain text table |
| json | `-o json` | JSON array |
| jsonl | `-o jsonl` | JSON Lines (one object per line) |
| csv | `-o csv` | Comma-separated values |
| tsv | `-o tsv` | Tab-separated values |
| yaml | `-o yaml` | YAML format |
| paths | `-o paths` | File paths only (one per line) |
| markdown | `-o markdown` | Markdown table |
| template | `-o template` | Custom Go template |

### Custom Templates

Use Go templates for custom output:

```bash
sweep -o template --template '{{.Path}}: {{.SizeHuman}}' ~/Downloads
```

Available template fields:
- `{{.Path}}` - Full file path
- `{{.Name}}` - File name
- `{{.Dir}}` - Parent directory
- `{{.Ext}}` - File extension
- `{{.Size}}` - Size in bytes
- `{{.SizeHuman}}` - Human-readable size
- `{{.ModTime}}` - Modification time
- `{{.Age}}` - Age as duration

### Filtering

```bash
# By age
sweep --older-than 30d .          # Files older than 30 days
sweep --newer-than 1w .           # Files from last week

# By type
sweep --type video .              # Video files
sweep --type "video,audio" .      # Multiple types

# By extension
sweep --ext ".mp4,.mkv" .         # Specific extensions

# By path pattern
sweep --include "**/Downloads/*" .

# Combine filters
sweep --type video --older-than 90d --ext .mkv ~/Videos
```

**File type groups:**
- `video`: .mp4, .mkv, .avi, .mov, .webm, etc.
- `audio`: .mp3, .wav, .flac, .ogg, .aac, etc.
- `image`: .png, .jpg, .gif, .svg, .webp, etc.
- `archive`: .zip, .tar, .gz, .rar, .7z, etc.
- `document`: .pdf, .doc, .docx, .xls, etc.
- `code`: .go, .py, .js, .ts, .rs, etc.
- `log`: .log, .out, .err

### Sorting

```bash
sweep --sort size .               # By size (default, largest first)
sweep --sort age .                # By age (oldest first)
sweep --sort path .               # Alphabetically by path
sweep --reverse .                 # Reverse sort order
```

### Limiting Results

```bash
sweep -l 10 .                     # Top 10 largest files
sweep --limit 0 .                 # Unlimited (all files)
```

## Command Reference

```
sweep [flags] [path]

Flags:
  -s, --min-size string      Minimum file size (default "100M")
  -e, --exclude strings      Exclude patterns
  -n, --no-interactive       Disable TUI
  -d, --dry-run              Preview only, don't delete
  -o, --output string        Output format
  -l, --limit int            Max files to return (default 50)
      --older-than string    Files older than duration
      --newer-than string    Files newer than duration
      --type string          File type groups
      --ext string           File extensions
      --sort string          Sort by: size, age, path
      --reverse              Reverse sort order
      --no-daemon            Bypass daemon
  -v, --verbose              Debug output
  -q, --quiet                Minimal output
  -h, --help                 Help
```

## Configuration

Configuration file location: `~/.config/sweep/config.yaml`

```yaml
# Minimum file size to consider "large"
min_size: 100M

# Default scan path (when no argument provided)
default_path: "."

# Patterns to exclude from scanning
exclude:
  - .git
  - node_modules
  - .cache
  - "*.swp"
  - "*.tmp"

# Worker configuration (auto-detected if not set)
workers:
  dir: 4
  file: 8

# Logging configuration
logging:
  level: info
  path: ~/.local/state/sweep/sweep.log

# Daemon configuration
daemon:
  auto_start: true
  socket_path: ~/.local/state/sweep/sweep.sock
  pid_path: ~/.local/state/sweep/sweep.pid
```

## Daemon

The sweep daemon (`sweepd`) maintains a persistent index of large files and watches for changes. It starts automatically when sweep runs (if `daemon.auto_start` is true in config).

### Manual Daemon Control

```bash
# Start daemon
sweep daemon start

# Stop daemon
sweep daemon stop

# Check status
sweep daemon status
```

### Daemon Benefits

- Instant results for previously scanned paths
- Real-time file change detection
- Background indexing while you work

### Bypassing the Daemon

```bash
sweep --no-daemon ~/Downloads     # Direct scan, ignore daemon
sweep --force-scan ~/Downloads    # Force direct scan
```

## Tips

**Find abandoned downloads:**
```bash
sweep --type archive --older-than 90d ~/Downloads
```

**Clean up old logs:**
```bash
sweep --type log --older-than 30d /var/log
```

**Export for scripting:**
```bash
sweep -o paths ~/Downloads | xargs -I {} trash {}
```

**Check specific extensions:**
```bash
sweep --ext ".iso,.dmg" -l 0 ~/Downloads
```
