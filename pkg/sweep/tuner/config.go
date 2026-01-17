package tuner

// Worker configuration limits.
const (
	// maxWorkers is the maximum number of workers for any pool.
	maxWorkers = 64

	// minDirWorkers is the minimum number of directory workers.
	// Directory traversal benefits from parallelism even on small systems.
	minDirWorkers = 8

	// minFileWorkers is the minimum number of file workers.
	minFileWorkers = 4

	// minQueueSize is the minimum queue/buffer size.
	minQueueSize = 100

	// maxQueueSize is the maximum queue/buffer size.
	maxQueueSize = 100000
)

// Memory-based queue sizing constants.
const (
	// bytesPerQueueEntry estimates memory per queue entry.
	// Each entry is roughly a path string (~256 bytes) plus metadata.
	bytesPerQueueEntry = 512

	// queueMemoryFraction is the fraction of available RAM to use for queues.
	// We use a small fraction since the actual files consume most memory.
	queueMemoryFraction = 0.05
)

// OptimalConfig contains tuned worker configuration optimized for the
// detected system resources.
type OptimalConfig struct {
	// DirWorkers is the number of directory walking workers.
	// Higher values improve directory traversal throughput.
	DirWorkers int

	// FileWorkers is the number of file processing workers.
	// Higher values improve I/O throughput on systems with fast storage.
	FileWorkers int

	// DirQueueSize is the directory queue buffer size.
	DirQueueSize int

	// FileQueueSize is the file queue buffer size.
	FileQueueSize int

	// ResultBuffer is the result buffer size.
	ResultBuffer int
}

// Calculate returns optimal configuration based on system resources.
//
// The calculation logic:
//   - DirWorkers: max(NumCPU, 8) - directory traversal is metadata-heavy
//     and benefits from parallelism
//   - FileWorkers: NumCPU * 4 - aggressive for I/O bound work since file
//     operations spend most time waiting on disk
//   - Both worker counts are capped at 64 to avoid excessive context switching
//   - Queue sizes are calculated based on available RAM
func Calculate(resources SystemResources) OptimalConfig {
	// Calculate directory workers: at least minDirWorkers or NumCPU
	dirWorkers := max(resources.CPUCores, minDirWorkers)
	dirWorkers = min(dirWorkers, maxWorkers)

	// Calculate file workers: NumCPU * 4 for I/O bound work
	fileWorkers := resources.CPUCores * 4
	fileWorkers = max(fileWorkers, minFileWorkers)
	fileWorkers = min(fileWorkers, maxWorkers)

	// Calculate queue sizes based on available memory
	queueSize := calculateQueueSize(resources.AvailableRAM)

	return OptimalConfig{
		DirWorkers:    dirWorkers,
		FileWorkers:   fileWorkers,
		DirQueueSize:  queueSize,
		FileQueueSize: queueSize,
		ResultBuffer:  queueSize,
	}
}

// CalculateWithOverrides applies user overrides to the optimal config.
// If workerOverride is greater than 0, it sets both DirWorkers and FileWorkers
// to that value (still respecting the maximum cap of 64).
// If workerOverride is 0 or negative, the default calculated values are used.
func CalculateWithOverrides(resources SystemResources, workerOverride int) OptimalConfig {
	config := Calculate(resources)

	if workerOverride > 0 {
		workers := min(workerOverride, maxWorkers)
		config.DirWorkers = workers
		config.FileWorkers = workers
	}

	return config
}

// calculateQueueSize determines queue size based on available memory.
func calculateQueueSize(availableRAM int64) int {
	// Calculate how much memory we can dedicate to queues
	queueMemory := float64(availableRAM) * queueMemoryFraction

	// Calculate number of entries that would fit
	entries := int(queueMemory / bytesPerQueueEntry)

	// Divide by 3 since we have 3 queues (dir, file, result)
	entriesPerQueue := entries / 3

	// Apply bounds
	entriesPerQueue = max(entriesPerQueue, minQueueSize)
	entriesPerQueue = min(entriesPerQueue, maxQueueSize)

	return entriesPerQueue
}
