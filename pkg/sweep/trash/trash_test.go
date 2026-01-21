package trash

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveToTrash(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0644))

	err := MoveToTrash(tmpFile)
	require.NoError(t, err)

	// File should no longer exist at original path
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveToTrash_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir")
	require.NoError(t, os.Mkdir(testDir, 0755))

	// Create a file inside the directory
	testFile := filepath.Join(testDir, "file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	err := MoveToTrash(testDir)
	require.NoError(t, err)

	// Directory should no longer exist at original path
	_, err = os.Stat(testDir)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveToTrash_NonexistentFile(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "nonexistent.txt")

	err := MoveToTrash(nonexistent)
	assert.Error(t, err)
}

func TestMoveToTrash_AbsolutePath(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "abs_test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("absolute path test"), 0644))

	// Ensure path is absolute
	absPath, err := filepath.Abs(tmpFile)
	require.NoError(t, err)

	err = MoveToTrash(absPath)
	require.NoError(t, err)

	_, err = os.Stat(absPath)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveToTrashMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	tmpFile := filepath.Join(t.TempDir(), "macos_test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("macos test"), 0644))

	err := moveToTrashMacOS(tmpFile)
	require.NoError(t, err)

	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveToTrashLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	tmpFile := filepath.Join(t.TempDir(), "linux_test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("linux test"), 0644))

	err := moveToTrashLinux(tmpFile)
	require.NoError(t, err)

	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

func TestFallbackDelete(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "fallback_test.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("fallback test"), 0644))

	err := fallbackDelete(tmpFile)
	require.NoError(t, err)

	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

func TestFallbackDelete_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "fallback_dir")
	require.NoError(t, os.Mkdir(testDir, 0755))

	// Create nested content
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("content"), 0644))

	err := fallbackDelete(testDir)
	require.NoError(t, err)

	_, err = os.Stat(testDir)
	assert.True(t, os.IsNotExist(err))
}
