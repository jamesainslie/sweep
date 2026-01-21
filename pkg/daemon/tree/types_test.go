package tree_test

import (
	"testing"

	"github.com/jamesainslie/sweep/pkg/daemon/tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTreeNode(t *testing.T) {
	t.Run("creates root directory node", func(t *testing.T) {
		root := &tree.Node{
			Path:  "/home/user/project",
			Name:  "project",
			IsDir: true,
		}

		assert.Equal(t, "/home/user/project", root.Path)
		assert.Equal(t, "project", root.Name)
		assert.True(t, root.IsDir)
		assert.Nil(t, root.Parent)
		assert.Empty(t, root.Children)
	})

	t.Run("creates file node with metadata", func(t *testing.T) {
		file := &tree.Node{
			Path:     "/home/user/project/main.go",
			Name:     "main.go",
			IsDir:    false,
			Size:     1024,
			ModTime:  1705600000,
			FileType: "go",
		}

		assert.Equal(t, "/home/user/project/main.go", file.Path)
		assert.Equal(t, "main.go", file.Name)
		assert.False(t, file.IsDir)
		assert.Equal(t, int64(1024), file.Size)
		assert.Equal(t, int64(1705600000), file.ModTime)
		assert.Equal(t, "go", file.FileType)
	})

	t.Run("AddChild establishes parent-child relationship", func(t *testing.T) {
		root := &tree.Node{
			Path:  "/home/user/project",
			Name:  "project",
			IsDir: true,
		}

		child := &tree.Node{
			Path:  "/home/user/project/src",
			Name:  "src",
			IsDir: true,
		}

		root.AddChild(child)

		require.Len(t, root.Children, 1)
		assert.Same(t, child, root.Children[0])
		assert.Same(t, root, child.Parent)
	})

	t.Run("AddChild with multiple children", func(t *testing.T) {
		root := &tree.Node{
			Path:  "/home/user/project",
			Name:  "project",
			IsDir: true,
		}

		src := &tree.Node{Path: "/home/user/project/src", Name: "src", IsDir: true}
		readme := &tree.Node{Path: "/home/user/project/README.md", Name: "README.md", IsDir: false}
		goMod := &tree.Node{Path: "/home/user/project/go.mod", Name: "go.mod", IsDir: false}

		root.AddChild(src)
		root.AddChild(readme)
		root.AddChild(goMod)

		require.Len(t, root.Children, 3)
		for _, child := range root.Children {
			assert.Same(t, root, child.Parent)
		}
	})

	t.Run("Depth returns correct values", func(t *testing.T) {
		// Build tree: root -> src -> internal -> file.go
		root := &tree.Node{Path: "/project", Name: "project", IsDir: true}
		src := &tree.Node{Path: "/project/src", Name: "src", IsDir: true}
		internal := &tree.Node{Path: "/project/src/internal", Name: "internal", IsDir: true}
		file := &tree.Node{Path: "/project/src/internal/file.go", Name: "file.go", IsDir: false}

		root.AddChild(src)
		src.AddChild(internal)
		internal.AddChild(file)

		assert.Equal(t, 0, root.Depth(), "root should be depth 0")
		assert.Equal(t, 1, src.Depth(), "src should be depth 1")
		assert.Equal(t, 2, internal.Depth(), "internal should be depth 2")
		assert.Equal(t, 3, file.Depth(), "file should be depth 3")
	})

	t.Run("IsLeaf returns true for files", func(t *testing.T) {
		file := &tree.Node{
			Path:  "/project/main.go",
			Name:  "main.go",
			IsDir: false,
		}

		assert.True(t, file.IsLeaf())
	})

	t.Run("IsLeaf returns true for empty directories", func(t *testing.T) {
		emptyDir := &tree.Node{
			Path:  "/project/empty",
			Name:  "empty",
			IsDir: true,
		}

		assert.True(t, emptyDir.IsLeaf())
	})

	t.Run("IsLeaf returns false for directories with children", func(t *testing.T) {
		dir := &tree.Node{
			Path:  "/project/src",
			Name:  "src",
			IsDir: true,
		}
		child := &tree.Node{
			Path:  "/project/src/main.go",
			Name:  "main.go",
			IsDir: false,
		}

		dir.AddChild(child)

		assert.False(t, dir.IsLeaf())
	})

	t.Run("directory with large file aggregates", func(t *testing.T) {
		dir := &tree.Node{
			Path:           "/project/assets",
			Name:           "assets",
			IsDir:          true,
			LargeFileSize:  500 * 1024 * 1024, // 500MB
			LargeFileCount: 5,
		}

		assert.Equal(t, "/project/assets", dir.Path)
		assert.Equal(t, "assets", dir.Name)
		assert.True(t, dir.IsDir)
		assert.Equal(t, int64(500*1024*1024), dir.LargeFileSize)
		assert.Equal(t, 5, dir.LargeFileCount)
	})

	t.Run("UI state fields", func(t *testing.T) {
		node := &tree.Node{
			Path:     "/project/src",
			Name:     "src",
			IsDir:    true,
			Expanded: true,
			Selected: true,
		}

		assert.Equal(t, "/project/src", node.Path)
		assert.Equal(t, "src", node.Name)
		assert.True(t, node.IsDir)
		assert.True(t, node.Expanded)
		assert.True(t, node.Selected)
	})
}
