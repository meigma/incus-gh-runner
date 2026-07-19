// Package pressure applies bounded guest-side CPU, memory, and disk pressure.
package pressure

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// MaxDuration is the longest pressure run accepted by Run.
	MaxDuration = 15 * time.Minute
	// MaxCPUWorkers is the largest CPU worker count accepted by Run.
	MaxCPUWorkers = 64
	// MaxMemoryBytes is the largest memory allocation accepted by Run.
	MaxMemoryBytes int64 = 4 << 30
	// MaxDiskBytes is the largest pressure file accepted by Run.
	MaxDiskBytes int64 = 4 << 30
	// MaxDiskBlockBytes is the largest synchronous disk write accepted by Run.
	MaxDiskBlockBytes int64 = 1 << 20
	// DefaultDiskBlockBytes is the synchronous write size used when none is supplied.
	DefaultDiskBlockBytes int64 = 256 << 10

	cpuHashBatchSize             uint64 = 256
	memoryCancellationCheckPages        = 256
	diskPatternSeed              uint64 = 0x9e3779b97f4a7c15
	diskPatternFirstLeftShift           = 13
	diskPatternRightShift               = 7
	diskPatternSecondLeftShift          = 17
)

var (
	// ErrInvalidOptions identifies an unsafe or incomplete pressure configuration.
	ErrInvalidOptions = errors.New("invalid pressure options")
)

// Options selects the bounded workloads applied by Run.
type Options struct {
	// Duration is how long the selected workloads run before successful completion.
	Duration time.Duration
	// CPUWorkers is the number of concurrent SHA-256 workers; zero disables CPU pressure.
	CPUWorkers int
	// MemoryBytes is the anonymous memory held and repeatedly touched; zero disables memory pressure.
	MemoryBytes int64
	// DiskPath is the absolute path of a new pressure file that remains for caller cleanup or evidence.
	DiskPath string
	// DiskBytes is the maximum pressure-file size; zero disables disk pressure.
	DiskBytes int64
	// DiskBlockBytes is the synchronous write size; zero selects DefaultDiskBlockBytes.
	DiskBlockBytes int64
}

// Result reports the work completed before the duration elapsed or cancellation occurred.
type Result struct {
	// CPUWorkers is the configured number of concurrent hashing workers.
	CPUWorkers int `json:"cpu_workers"`
	// CPUHashes is the number of SHA-256 operations completed by all CPU workers.
	CPUHashes uint64 `json:"cpu_hashes"`
	// MemoryBytes is the amount of anonymous memory held for the run.
	MemoryBytes int64 `json:"memory_bytes"`
	// DiskTargetBytes is the maximum size requested for the pressure file.
	DiskTargetBytes int64 `json:"disk_target_bytes"`
	// DiskBytesWritten is the cumulative number of synchronous bytes written.
	DiskBytesWritten int64 `json:"disk_bytes_written"`
	// DiskFileBytes is the final pressure-file size, which never exceeds Options.DiskBytes.
	DiskFileBytes int64 `json:"disk_file_bytes"`
	// Elapsed is the wall-clock time spent applying pressure after setup completed.
	Elapsed time.Duration `json:"elapsed"`
}

// normalizedOptions contains validated values converted for allocation and I/O.
type normalizedOptions struct {
	duration       time.Duration
	cpuWorkers     int
	memoryBytes    int
	diskPath       string
	diskBytes      int64
	diskBlockBytes int
}

// pressureCounters accumulates results without adding locks to hot workload loops.
type pressureCounters struct {
	cpuHashes        atomic.Uint64
	diskBytesWritten atomic.Int64
}

// Run applies the selected pressure until Duration elapses or ctx is canceled.
//
// Run returns partial counters with context cancellation errors. Disk pressure
// uses synchronous writes and refuses to overwrite an existing path.
func Run(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("%w: context is nil", ErrInvalidOptions)
	}

	config, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return Result{}, contextErr
	}

	var diskFile *os.File
	if config.diskBytes > 0 {
		diskFile, err = openDiskFile(config.diskPath)
		if err != nil {
			return Result{}, err
		}
	}

	memory := make([]byte, config.memoryBytes)
	counters := &pressureCounters{}
	runCtx, cancel := context.WithCancel(ctx)
	timer := time.NewTimer(config.duration)
	workerErrors := make(chan error, 1)
	startedAt := time.Now()

	workers := startWorkers(runCtx, config, diskFile, memory, counters, workerErrors)
	runErr := waitForCompletion(ctx, timer, workerErrors)

	cancel()
	workers.Wait()
	stopTimer(timer)
	runErr = joinWorkerError(runErr, workerErrors)

	result, resultErr := collectResult(config, counters, len(memory), time.Since(startedAt))
	runtime.KeepAlive(memory)
	return result, errors.Join(runErr, resultErr)
}

// startWorkers starts each enabled workload and returns their shared lifecycle boundary.
func startWorkers(
	ctx context.Context,
	config normalizedOptions,
	diskFile *os.File,
	memory []byte,
	counters *pressureCounters,
	workerErrors chan<- error,
) *sync.WaitGroup {
	workers := &sync.WaitGroup{}
	for workerID, remaining := uint64(1), config.cpuWorkers; remaining > 0; workerID, remaining = workerID+1, remaining-1 {
		startCPUWorker(ctx, workers, workerID, &counters.cpuHashes)
	}
	if len(memory) > 0 {
		workers.Go(func() {
			runMemoryWorker(ctx, memory)
		})
	}
	if diskFile != nil {
		workers.Go(func() {
			if err := runDiskWorker(ctx, diskFile, config, &counters.diskBytesWritten); err != nil {
				signalError(workerErrors, err)
			}
		})
	}
	return workers
}

// startCPUWorker starts one hashing worker with a stable numeric seed.
func startCPUWorker(ctx context.Context, workers *sync.WaitGroup, workerID uint64, hashes *atomic.Uint64) {
	workers.Go(func() {
		runCPUWorker(ctx, workerID, hashes)
	})
}

// waitForCompletion waits for natural completion, caller cancellation, or a worker failure.
func waitForCompletion(ctx context.Context, timer *time.Timer, workerErrors <-chan error) error {
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case workerErr := <-workerErrors:
		return workerErr
	}
}

// stopTimer releases the duration timer and drains a concurrent expiration.
func stopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

// joinWorkerError adds a worker failure reported during coordinated shutdown.
func joinWorkerError(runErr error, workerErrors <-chan error) error {
	select {
	case workerErr := <-workerErrors:
		return errors.Join(runErr, workerErr)
	default:
		return runErr
	}
}

// normalizeOptions validates hard workload bounds and applies disk defaults.
func normalizeOptions(opts Options) (normalizedOptions, error) {
	if err := validateWorkloadBounds(opts); err != nil {
		return normalizedOptions{}, err
	}

	diskPath, diskBlockBytes, err := normalizeDiskOptions(opts)
	if err != nil {
		return normalizedOptions{}, err
	}
	return normalizedOptions{
		duration:       opts.Duration,
		cpuWorkers:     opts.CPUWorkers,
		memoryBytes:    int(opts.MemoryBytes),
		diskPath:       diskPath,
		diskBytes:      opts.DiskBytes,
		diskBlockBytes: diskBlockBytes,
	}, nil
}

// validateWorkloadBounds rejects incomplete configurations and values above hard safety limits.
func validateWorkloadBounds(opts Options) error {
	if opts.Duration <= 0 || opts.Duration > MaxDuration {
		return fmt.Errorf(
			"%w: duration must be between 1ns and %s",
			ErrInvalidOptions,
			MaxDuration,
		)
	}
	if opts.CPUWorkers < 0 || opts.CPUWorkers > MaxCPUWorkers {
		return fmt.Errorf("%w: CPU workers must be between 0 and %d", ErrInvalidOptions, MaxCPUWorkers)
	}
	if opts.MemoryBytes < 0 || opts.MemoryBytes > MaxMemoryBytes {
		return fmt.Errorf("%w: memory bytes must be between 0 and %d", ErrInvalidOptions, MaxMemoryBytes)
	}
	if opts.MemoryBytes > int64(^uint(0)>>1) {
		return fmt.Errorf("%w: memory bytes exceed the platform allocation limit", ErrInvalidOptions)
	}
	if opts.DiskBytes < 0 || opts.DiskBytes > MaxDiskBytes {
		return fmt.Errorf("%w: disk bytes must be between 0 and %d", ErrInvalidOptions, MaxDiskBytes)
	}
	if opts.DiskBlockBytes < 0 || opts.DiskBlockBytes > MaxDiskBlockBytes {
		return fmt.Errorf("%w: disk block bytes must be between 0 and %d", ErrInvalidOptions, MaxDiskBlockBytes)
	}
	if opts.CPUWorkers == 0 && opts.MemoryBytes == 0 && opts.DiskBytes == 0 {
		return fmt.Errorf("%w: at least one workload must be enabled", ErrInvalidOptions)
	}
	return nil
}

// normalizeDiskOptions validates the disk contract and resolves its effective block size.
func normalizeDiskOptions(opts Options) (string, int, error) {
	if opts.DiskBytes == 0 {
		if opts.DiskPath != "" || opts.DiskBlockBytes != 0 {
			return "", 0, fmt.Errorf("%w: disk path and block size require disk pressure", ErrInvalidOptions)
		}
		return "", 0, nil
	}
	if opts.DiskPath == "" || !filepath.IsAbs(opts.DiskPath) {
		return "", 0, fmt.Errorf("%w: disk path must be absolute when disk pressure is enabled", ErrInvalidOptions)
	}

	blockBytes := opts.DiskBlockBytes
	if blockBytes == 0 {
		blockBytes = DefaultDiskBlockBytes
	}
	if blockBytes > opts.DiskBytes {
		blockBytes = opts.DiskBytes
	}
	return filepath.Clean(opts.DiskPath), int(blockBytes), nil
}

// openDiskFile creates a private pressure file without following an existing path.
func openDiskFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_SYNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create pressure file %q: %w", path, err)
	}
	return file, nil
}

// runCPUWorker hashes a changing seed in bounded batches until cancellation.
func runCPUWorker(ctx context.Context, workerID uint64, hashes *atomic.Uint64) {
	var seed [sha256.Size]byte
	binary.LittleEndian.PutUint64(seed[:], workerID)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		for range cpuHashBatchSize {
			seed = sha256.Sum256(seed[:])
		}
		hashes.Add(cpuHashBatchSize)
	}
}

// runMemoryWorker repeatedly touches each allocated page until cancellation.
func runMemoryWorker(ctx context.Context, memory []byte) {
	for touchMemoryPass(ctx, memory) {
	}
}

// touchMemoryPass mutates every allocated page and reports whether work should continue.
func touchMemoryPass(ctx context.Context, memory []byte) bool {
	pageBytes := os.Getpagesize()
	for offset := 0; offset < len(memory); offset += pageBytes {
		if offset%(pageBytes*memoryCancellationCheckPages) == 0 {
			select {
			case <-ctx.Done():
				return false
			default:
			}
		}
		memory[offset]++
	}
	return true
}

// runDiskWorker synchronously rewrites a bounded file and closes it after cancellation.
func runDiskWorker(ctx context.Context, file *os.File, config normalizedOptions, written *atomic.Int64) error {
	runErr := writeDiskPressure(ctx, file, config, written)
	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close pressure file %q: %w", config.diskPath, closeErr)
	}
	return errors.Join(runErr, closeErr)
}

// writeDiskPressure performs synchronous bounded rewrites until cancellation.
func writeDiskPressure(ctx context.Context, file *os.File, config normalizedOptions, written *atomic.Int64) error {
	block := make([]byte, config.diskBlockBytes)
	fillDiskBlock(block)
	var offset int64
	for {
		select {
		case <-ctx.Done():
			if syncErr := file.Sync(); syncErr != nil {
				return fmt.Errorf("sync pressure file %q: %w", config.diskPath, syncErr)
			}
			return nil
		default:
		}

		remaining := config.diskBytes - offset
		writeBytes := min(int64(len(block)), remaining)
		block[0]++
		block[len(block)-1]--
		n, writeErr := file.Write(block[:int(writeBytes)])
		written.Add(int64(n))
		if writeErr != nil {
			return fmt.Errorf("write pressure file %q: %w", config.diskPath, writeErr)
		}
		if int64(n) != writeBytes {
			return fmt.Errorf("write pressure file %q: %w", config.diskPath, io.ErrShortWrite)
		}

		offset += int64(n)
		if offset == config.diskBytes {
			if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
				return fmt.Errorf("rewind pressure file %q: %w", config.diskPath, seekErr)
			}
			offset = 0
		}
	}
}

// fillDiskBlock fills a block with deterministic data that resists filesystem compression.
func fillDiskBlock(block []byte) {
	state := diskPatternSeed
	var encoded [8]byte
	for offset := 0; offset < len(block); offset += len(encoded) {
		state ^= state << diskPatternFirstLeftShift
		state ^= state >> diskPatternRightShift
		state ^= state << diskPatternSecondLeftShift
		binary.LittleEndian.PutUint64(encoded[:], state)
		copy(block[offset:], encoded[:])
	}
}

// signalError publishes the first worker error without blocking shutdown.
func signalError(target chan<- error, err error) {
	select {
	case target <- err:
	default:
	}
}

// collectResult snapshots counters and the bounded pressure-file size.
func collectResult(
	config normalizedOptions,
	counters *pressureCounters,
	memoryBytes int,
	elapsed time.Duration,
) (Result, error) {
	result := Result{
		CPUWorkers:       config.cpuWorkers,
		CPUHashes:        counters.cpuHashes.Load(),
		MemoryBytes:      int64(memoryBytes),
		DiskTargetBytes:  config.diskBytes,
		DiskBytesWritten: counters.diskBytesWritten.Load(),
		Elapsed:          elapsed,
	}
	if config.diskPath == "" {
		return result, nil
	}

	info, err := os.Stat(config.diskPath)
	if err != nil {
		return result, fmt.Errorf("stat pressure file %q: %w", config.diskPath, err)
	}
	result.DiskFileBytes = info.Size()
	if result.DiskFileBytes > config.diskBytes {
		return result, fmt.Errorf("pressure file %q exceeded its %d-byte limit", config.diskPath, config.diskBytes)
	}
	return result, nil
}
