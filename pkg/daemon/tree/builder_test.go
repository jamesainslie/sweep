package tree_test

import (
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTree(t *testing.T) {
	t.Run("builds tree from list of large files", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/src/main.go", Size: 1000, ModTime: 1705600000},
			{Path: "/project/src/utils.go", Size: 2000, ModTime: 1705600001},
			{Path: "/project/assets/image.png", Size: 5000, ModTime: 1705600002},
		}

		root := tree.BuildTree("/project", files, 0)

		require.NotNil(t, root)
		assert.Equal(t, "/project", root.Path)
		assert.Equal(t, "project", root.Name)
		assert.True(t, root.IsDir)
		assert.Equal(t, int64(8000), root.LargeFileSize)
		assert.Equal(t, 3, root.LargeFileCount)
	})

	t.Run("creates correct directory structure", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/src/internal/handler.go", Size: 1000, ModTime: 1705600000},
			{Path: "/project/src/main.go", Size: 2000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 0)

		require.NotNil(t, root)
		require.Len(t, root.Children, 1, "root should have one child (src)")

		src := root.Children[0]
		assert.Equal(t, "/project/src", src.Path)
		assert.Equal(t, "src", src.Name)
		assert.True(t, src.IsDir)
		assert.Equal(t, int64(3000), src.LargeFileSize)
		assert.Equal(t, 2, src.LargeFileCount)

		// src should have 2 children: internal dir and main.go file
		require.Len(t, src.Children, 2)
	})

	t.Run("sets file nodes with correct metadata", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/main.go", Size: 1500, ModTime: 1705600000},
		}

		root := tree.BuildTree("/project", files, 0)

		require.Len(t, root.Children, 1)
		file := root.Children[0]

		assert.Equal(t, "/project/main.go", file.Path)
		assert.Equal(t, "main.go", file.Name)
		assert.False(t, file.IsDir)
		assert.Equal(t, int64(1500), file.Size)
		assert.Equal(t, int64(1705600000), file.ModTime)
		assert.Equal(t, "Go", file.FileType)
	})

	t.Run("filters files below minSize", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/small.go", Size: 100, ModTime: 1705600000},
			{Path: "/project/large.go", Size: 1000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 500)

		assert.Equal(t, int64(1000), root.LargeFileSize)
		assert.Equal(t, 1, root.LargeFileCount)
		require.Len(t, root.Children, 1)
		assert.Equal(t, "large.go", root.Children[0].Name)
	})

	t.Run("handles empty file list", func(t *testing.T) {
		root := tree.BuildTree("/project", []tree.LargeFile{}, 0)

		require.NotNil(t, root)
		assert.Equal(t, "/project", root.Path)
		assert.Equal(t, "project", root.Name)
		assert.True(t, root.IsDir)
		assert.Equal(t, int64(0), root.LargeFileSize)
		assert.Equal(t, 0, root.LargeFileCount)
		assert.Empty(t, root.Children)
	})

	t.Run("handles root path with trailing slash", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/main.go", Size: 1000, ModTime: 1705600000},
		}

		root := tree.BuildTree("/project/", files, 0)

		assert.Equal(t, "/project", root.Path)
		require.Len(t, root.Children, 1)
		assert.Equal(t, "/project/main.go", root.Children[0].Path)
	})
}

func TestBuildTreeHidesEmptyDirs(t *testing.T) {
	t.Run("only includes directories with large files", func(t *testing.T) {
		// Files only in src/internal, not in assets
		files := []tree.LargeFile{
			{Path: "/project/src/internal/handler.go", Size: 1000, ModTime: 1705600000},
			{Path: "/project/src/internal/utils.go", Size: 2000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 0)

		require.NotNil(t, root)
		require.Len(t, root.Children, 1, "root should only have src, not assets")

		src := root.Children[0]
		assert.Equal(t, "src", src.Name)
		require.Len(t, src.Children, 1, "src should only have internal")

		internal := src.Children[0]
		assert.Equal(t, "internal", internal.Name)
		require.Len(t, internal.Children, 2, "internal should have both files")
	})

	t.Run("excludes directories when all files filtered by minSize", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/src/small.go", Size: 100, ModTime: 1705600000},
			{Path: "/project/assets/large.png", Size: 5000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 1000)

		require.Len(t, root.Children, 1, "should only have assets dir")
		assert.Equal(t, "assets", root.Children[0].Name)
	})
}

func TestBuildTreeSortsBySize(t *testing.T) {
	t.Run("sorts directories by LargeFileSize descending", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/small/a.go", Size: 100, ModTime: 1705600000},
			{Path: "/project/medium/b.go", Size: 500, ModTime: 1705600001},
			{Path: "/project/large/c.go", Size: 1000, ModTime: 1705600002},
		}

		root := tree.BuildTree("/project", files, 0)

		require.Len(t, root.Children, 3)
		assert.Equal(t, "large", root.Children[0].Name, "largest dir should be first")
		assert.Equal(t, "medium", root.Children[1].Name, "medium dir should be second")
		assert.Equal(t, "small", root.Children[2].Name, "smallest dir should be last")
	})

	t.Run("sorts files by Size descending", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/small.go", Size: 100, ModTime: 1705600000},
			{Path: "/project/large.go", Size: 1000, ModTime: 1705600001},
			{Path: "/project/medium.go", Size: 500, ModTime: 1705600002},
		}

		root := tree.BuildTree("/project", files, 0)

		require.Len(t, root.Children, 3)
		assert.Equal(t, "large.go", root.Children[0].Name, "largest file should be first")
		assert.Equal(t, "medium.go", root.Children[1].Name, "medium file should be second")
		assert.Equal(t, "small.go", root.Children[2].Name, "smallest file should be last")
	})

	t.Run("directories sorted before files at same level with same size", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/file.go", Size: 1000, ModTime: 1705600000},
			{Path: "/project/dir/nested.go", Size: 1000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 0)

		require.Len(t, root.Children, 2)
		assert.True(t, root.Children[0].IsDir, "directory should come before file")
		assert.False(t, root.Children[1].IsDir, "file should come after directory")
	})

	t.Run("sorts nested children recursively", func(t *testing.T) {
		files := []tree.LargeFile{
			{Path: "/project/src/small.go", Size: 100, ModTime: 1705600000},
			{Path: "/project/src/large.go", Size: 1000, ModTime: 1705600001},
		}

		root := tree.BuildTree("/project", files, 0)

		require.Len(t, root.Children, 1)
		src := root.Children[0]
		require.Len(t, src.Children, 2)
		assert.Equal(t, "large.go", src.Children[0].Name, "larger file should be first in nested dir")
		assert.Equal(t, "small.go", src.Children[1].Name, "smaller file should be last in nested dir")
	})
}

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		// Programming languages
		{"/project/main.go", "Go"},
		{"/project/app.py", "Python"},
		{"/project/index.js", "JavaScript"},
		{"/project/app.ts", "TypeScript"},
		{"/project/main.rs", "Rust"},
		{"/project/main.c", "C"},
		{"/project/main.cpp", "C++"},
		{"/project/main.cc", "C++"},
		{"/project/App.java", "Java"},
		{"/project/app.rb", "Ruby"},
		{"/project/script.sh", "Shell"},
		{"/project/script.bash", "Shell"},
		{"/project/script.zsh", "Shell"},

		// Web
		{"/project/index.html", "HTML"},
		{"/project/styles.css", "CSS"},
		{"/project/app.jsx", "JSX"},
		{"/project/app.tsx", "TSX"},
		{"/project/App.vue", "Vue"},
		{"/project/App.svelte", "Svelte"},

		// Data/Config
		{"/project/data.json", "JSON"},
		{"/project/config.yaml", "YAML"},
		{"/project/config.yml", "YAML"},
		{"/project/config.toml", "TOML"},
		{"/project/data.xml", "XML"},
		{"/project/data.csv", "CSV"},

		// Documentation
		{"/project/README.md", "Markdown"},
		{"/project/doc.txt", "Text"},
		{"/project/doc.pdf", "PDF"},

		// Media
		{"/project/image.png", "Image"},
		{"/project/photo.jpg", "Image"},
		{"/project/photo.jpeg", "Image"},
		{"/project/icon.gif", "Image"},
		{"/project/icon.svg", "Image"},
		{"/project/icon.webp", "Image"},
		{"/project/video.mp4", "Video"},
		{"/project/video.mov", "Video"},
		{"/project/video.avi", "Video"},
		{"/project/audio.mp3", "Audio"},
		{"/project/audio.wav", "Audio"},

		// Archives
		{"/project/archive.zip", "Archive"},
		{"/project/archive.tar", "Archive"},
		{"/project/archive.gz", "Archive"},
		{"/project/archive.tar.gz", "Archive"},
		{"/project/archive.rar", "Archive"},
		{"/project/archive.7z", "Archive"},

		// Binaries/Executables
		{"/project/binary.exe", "Executable"},
		{"/project/app.dll", "Library"},
		{"/project/lib.so", "Library"},
		{"/project/lib.dylib", "Library"},
		{"/project/app.wasm", "WebAssembly"},

		// Database
		{"/project/data.db", "Database"},
		{"/project/data.sqlite", "Database"},
		{"/project/data.sqlite3", "Database"},

		// Unknown
		{"/project/unknown.xyz", "File"},
		{"/project/noextension", "File"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := tree.DetectFileType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
