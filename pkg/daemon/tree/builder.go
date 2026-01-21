package tree

import (
	"path/filepath"
	"sort"
	"strings"
)

// LargeFile represents a file that exceeds the size threshold.
type LargeFile struct {
	Path    string
	Size    int64
	ModTime int64
}

// BuildTree constructs a tree from a list of large files.
// Files smaller than minSize are excluded from the tree.
// The resulting tree only contains directories that have large files underneath.
// Children are sorted by size descending (directories by LargeFileSize, files by Size).
func BuildTree(root string, files []LargeFile, minSize int64) *Node {
	root = strings.TrimSuffix(root, "/")

	rootNode := &Node{
		Path:  root,
		Name:  filepath.Base(root),
		IsDir: true,
	}

	// Map of path -> node for quick lookup
	nodes := make(map[string]*Node)
	nodes[root] = rootNode

	// Filter and add files
	for _, f := range files {
		if f.Size < minSize {
			continue
		}

		// Ensure all ancestor directories exist
		ensureAncestors(root, f.Path, nodes)

		// Create file node
		fileNode := &Node{
			Path:     f.Path,
			Name:     filepath.Base(f.Path),
			IsDir:    false,
			Size:     f.Size,
			ModTime:  f.ModTime,
			FileType: DetectFileType(f.Path),
		}

		// Add to parent
		parentPath := filepath.Dir(f.Path)
		if parent, ok := nodes[parentPath]; ok {
			parent.AddChild(fileNode)
		}
		nodes[f.Path] = fileNode
	}

	// Aggregate sizes up the tree
	aggregateSizes(rootNode)

	// Sort children by size
	sortChildren(rootNode)

	return rootNode
}

// ensureAncestors creates all directory nodes between root and the file's parent.
func ensureAncestors(root, filePath string, nodes map[string]*Node) {
	parentPath := filepath.Dir(filePath)

	// Build list of directories to create (from file up to root)
	var dirsToCreate []string
	for parentPath != root && parentPath != "/" && parentPath != "." {
		if _, exists := nodes[parentPath]; !exists {
			dirsToCreate = append(dirsToCreate, parentPath)
		}
		parentPath = filepath.Dir(parentPath)
	}

	// Create directories in reverse order (from root down)
	for i := len(dirsToCreate) - 1; i >= 0; i-- {
		dirPath := dirsToCreate[i]
		dirNode := &Node{
			Path:  dirPath,
			Name:  filepath.Base(dirPath),
			IsDir: true,
		}

		// Find parent and add
		parentPath := filepath.Dir(dirPath)
		if parent, ok := nodes[parentPath]; ok {
			parent.AddChild(dirNode)
		}
		nodes[dirPath] = dirNode
	}
}

// aggregateSizes calculates LargeFileSize and LargeFileCount for all directories.
func aggregateSizes(node *Node) (totalSize int64, totalCount int) {
	if !node.IsDir {
		return node.Size, 1
	}

	for _, child := range node.Children {
		size, count := aggregateSizes(child)
		totalSize += size
		totalCount += count
	}

	node.LargeFileSize = totalSize
	node.LargeFileCount = totalCount

	return totalSize, totalCount
}

// sortChildren sorts all children recursively by size descending.
// Directories come before files when sizes are equal.
func sortChildren(node *Node) {
	if !node.IsDir || len(node.Children) == 0 {
		return
	}

	// Sort children
	sort.Slice(node.Children, func(i, j int) bool {
		a, b := node.Children[i], node.Children[j]

		// Get comparable size (LargeFileSize for dirs, Size for files)
		aSize := a.Size
		if a.IsDir {
			aSize = a.LargeFileSize
		}
		bSize := b.Size
		if b.IsDir {
			bSize = b.LargeFileSize
		}

		// Sort by size descending
		if aSize != bSize {
			return aSize > bSize
		}

		// Directories before files when same size
		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		// Alphabetical as tiebreaker
		return a.Name < b.Name
	})

	// Sort children recursively
	for _, child := range node.Children {
		sortChildren(child)
	}
}

// fileTypeMap maps file extensions to human-readable type names.
var fileTypeMap = map[string]string{
	// Programming languages
	".go":   "Go",
	".py":   "Python",
	".js":   "JavaScript",
	".ts":   "TypeScript",
	".rs":   "Rust",
	".c":    "C",
	".cpp":  "C++",
	".cc":   "C++",
	".cxx":  "C++",
	".java": "Java",
	".rb":   "Ruby",
	".sh":   "Shell",
	".bash": "Shell",
	".zsh":  "Shell",

	// Web
	".html":   "HTML",
	".htm":    "HTML",
	".css":    "CSS",
	".jsx":    "JSX",
	".tsx":    "TSX",
	".vue":    "Vue",
	".svelte": "Svelte",

	// Data/Config
	".json": "JSON",
	".yaml": "YAML",
	".yml":  "YAML",
	".toml": "TOML",
	".xml":  "XML",
	".csv":  "CSV",

	// Documentation
	".md":       "Markdown",
	".markdown": "Markdown",
	".txt":      "Text",
	".pdf":      "PDF",

	// Images
	".png":  "Image",
	".jpg":  "Image",
	".jpeg": "Image",
	".gif":  "Image",
	".svg":  "Image",
	".webp": "Image",
	".bmp":  "Image",
	".ico":  "Image",

	// Video
	".mp4":  "Video",
	".mov":  "Video",
	".avi":  "Video",
	".mkv":  "Video",
	".webm": "Video",

	// Audio
	".mp3":  "Audio",
	".wav":  "Audio",
	".ogg":  "Audio",
	".flac": "Audio",
	".aac":  "Audio",

	// Archives
	".zip": "Archive",
	".tar": "Archive",
	".gz":  "Archive",
	".rar": "Archive",
	".7z":  "Archive",
	".bz2": "Archive",
	".xz":  "Archive",

	// Executables and libraries
	".exe":   "Executable",
	".dll":   "Library",
	".so":    "Library",
	".dylib": "Library",
	".a":     "Library",
	".wasm":  "WebAssembly",

	// Database
	".db":      "Database",
	".sqlite":  "Database",
	".sqlite3": "Database",
}

// DetectFileType returns a human-readable file type based on the file extension.
func DetectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if fileType, ok := fileTypeMap[ext]; ok {
		return fileType
	}
	return "File"
}
