# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Unified header** across list and tree views with consistent elements:
  - App icon and title
  - File count and total size
  - "Freed X" indicator after deletions
  - "LIVE" indicator when daemon watching is active
  - Scan metrics line (directories/files scanned, elapsed time)
  - Key hints bar
  - Column headers

- **Percentage indicator** in tree view showing each item's size as a percentage of total large file bytes. Replaces the previous bar visualization for a cleaner, more readable display.

- **Directory selection** in tree view allowing entire directories to be marked for deletion.

- **Shared header rendering** extracted to `header.go` for code reuse between views.

- **Icon helper function** (`getNodeIcon`) for consistent icon selection in tree view based on node type, selection state, and expansion state.

### Changed

- **List view is now the default** when launching the TUI. Press `t` to switch to tree view.

- **Tree view starts collapsed** with only the root node expanded, allowing users to drill into areas of interest.

- **Tree icons simplified** to use single Unicode symbols per row:
  - Directories: filled triangles when selected, outline when not; direction indicates expand state
  - Files: filled circle when selected, outline when not

### Removed

- **Bar visualization** in tree view (e.g., `████░░░`) replaced with percentage indicator.

## [0.1.0] - 2026-01-17

### Added

- Initial release with TUI and non-interactive modes
- Daemon architecture with persistent indexing
- Real-time file watching via fsnotify
- Multiple output formats (JSON, CSV, YAML, Markdown, etc.)
- Filtering by age, type, extension, and path patterns
- Tree view with hierarchical directory navigation
- Safe deletion via system trash
