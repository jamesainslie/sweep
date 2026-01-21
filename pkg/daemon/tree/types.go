// Package tree provides types for hierarchical directory/file tree display.
package tree

// Node represents a directory or file in the tree.
type Node struct {
	// Identity
	Path string `json:"path"`
	Name string `json:"name"`

	// Type
	IsDir bool `json:"is_dir"`

	// For files
	Size     int64  `json:"size,omitempty"`
	ModTime  int64  `json:"mod_time,omitempty"`
	FileType string `json:"file_type,omitempty"`

	// For directories - aggregates of large files underneath
	LargeFileSize  int64 `json:"large_file_size,omitempty"`
	LargeFileCount int   `json:"large_file_count,omitempty"`

	// Tree structure
	Children []*Node `json:"children,omitempty"`
	Parent   *Node   `json:"-"` // Exclude from JSON to avoid cycles

	// UI state
	Expanded bool `json:"expanded,omitempty"`
	Selected bool `json:"selected,omitempty"`
}

// AddChild adds a child node and sets this node as the child's parent.
func (n *Node) AddChild(child *Node) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// IsLeaf returns true if the node is a file or an empty directory.
func (n *Node) IsLeaf() bool {
	return !n.IsDir || len(n.Children) == 0
}

// Depth returns the depth of this node from the root (root = 0).
func (n *Node) Depth() int {
	depth := 0
	current := n.Parent
	for current != nil {
		depth++
		current = current.Parent
	}
	return depth
}

// Flatten returns a slice of all visible nodes in display order.
// Collapsed directories hide their children.
func (n *Node) Flatten() []*Node {
	result := []*Node{n}

	// Only recurse into children if this is an expanded directory
	if n.IsDir && n.Expanded {
		for _, child := range n.Children {
			result = append(result, child.Flatten()...)
		}
	}

	return result
}

// Toggle expands or collapses a directory node.
// Has no effect on file nodes.
func (n *Node) Toggle() {
	if !n.IsDir {
		return
	}
	n.Expanded = !n.Expanded
}

// ExpandAll expands this node and all descendants.
// Only affects directory nodes.
func (n *Node) ExpandAll() {
	if n.IsDir {
		n.Expanded = true
		for _, child := range n.Children {
			child.ExpandAll()
		}
	}
}

// CollapseAll collapses this node and all descendants.
// Only affects directory nodes.
func (n *Node) CollapseAll() {
	if n.IsDir {
		n.Expanded = false
		for _, child := range n.Children {
			child.CollapseAll()
		}
	}
}
