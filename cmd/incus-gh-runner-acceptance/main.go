// Command incus-gh-runner-acceptance runs non-shipped disposable-host security probes.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/meigma/incus-gh-runner/internal/liveacceptance"
	"github.com/meigma/incus-gh-runner/internal/liveacceptance/pressure"
)

const defaultProbeStressDuration = 10 * time.Minute

// main executes the acceptance command and exits with its status code.
func main() {
	os.Exit(run())
}

// run builds and executes the signal-aware acceptance command tree.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	command := newRootCommand()
	if err := command.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// newRootCommand creates the non-shipped acceptance command tree.
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "incus-gh-runner-acceptance",
		Short:         "Run disposable Incus runtime security probes",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(newProbeCommand(), newGuestPressureCommand())
	return root
}

// probeFlags contains the operands bound by the parent hostile-runner harness.
type probeFlags struct {
	baselinePath      string
	project           string
	profile           string
	imageFingerprint  string
	vmA               string
	vmB               string
	runID             string
	allowedURL        string
	egressProxy       string
	evidenceDirectory string
	stressDuration    time.Duration
	pollInterval      time.Duration
}

// newProbeCommand creates the host-side live runtime probe command.
func newProbeCommand() *cobra.Command {
	flags := probeFlags{}
	command := &cobra.Command{
		Use:   "probe",
		Short: "Probe two marker-owned runner VMs on a disposable KVM host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selfPath, err := pressureExecutablePath()
			if err != nil {
				return err
			}
			_, err = liveacceptance.Run(cmd.Context(), liveacceptance.Options{
				BaselinePath:      flags.baselinePath,
				Project:           flags.project,
				Profile:           flags.profile,
				ImageFingerprint:  flags.imageFingerprint,
				VMA:               flags.vmA,
				VMB:               flags.vmB,
				RunID:             flags.runID,
				AllowedURL:        flags.allowedURL,
				EgressProxy:       flags.egressProxy,
				EvidenceDirectory: flags.evidenceDirectory,
				StressDuration:    flags.stressDuration,
				PollInterval:      flags.pollInterval,
				SelfPath:          selfPath,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"runtime acceptance passed; evidence written to %s\n",
				flags.evidenceDirectory,
			)
			return err
		},
	}
	command.Flags().StringVar(&flags.baselinePath, "baseline", "", "rendered CUE baseline path")
	command.Flags().StringVar(&flags.project, "project", "", "disposable runner project")
	command.Flags().StringVar(&flags.profile, "profile", "", "sole runner profile")
	command.Flags().StringVar(&flags.imageFingerprint, "image-fingerprint", "", "immutable runner image fingerprint")
	command.Flags().StringVar(&flags.vmA, "vm-a", "", "marker-owned aggressor VM")
	command.Flags().StringVar(&flags.vmB, "vm-b", "", "marker-owned victim VM")
	command.Flags().StringVar(&flags.runID, "run-id", "", "parent hostile harness run ID")
	command.Flags().StringVar(&flags.allowedURL, "allowed-url", "", "credential-free approved HTTP origin")
	command.Flags().StringVar(&flags.egressProxy, "egress-proxy", "", "optional credential-free HTTP proxy origin")
	command.Flags().StringVar(&flags.evidenceDirectory, "evidence-directory", "", "new private evidence directory")
	command.Flags().
		DurationVar(&flags.stressDuration, "stress-duration", defaultProbeStressDuration, "steady-state guest pressure duration")
	command.Flags().DurationVar(&flags.pollInterval, "poll-interval", time.Second, "runtime health sample interval")
	return command
}

// guestPressureFlags contains the bounded workload values supplied by the host-side probe.
type guestPressureFlags struct {
	duration       time.Duration
	cpuWorkers     int
	memoryBytes    int64
	diskPath       string
	diskBytes      int64
	diskBlockBytes int64
}

// newGuestPressureCommand creates the internal guest-side workload command.
func newGuestPressureCommand() *cobra.Command {
	flags := guestPressureFlags{}
	command := &cobra.Command{
		Use:    "guest-pressure",
		Short:  "Apply bounded CPU, memory, and disk pressure inside a disposable VM",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := pressure.Run(cmd.Context(), pressure.Options{
				Duration:       flags.duration,
				CPUWorkers:     flags.cpuWorkers,
				MemoryBytes:    flags.memoryBytes,
				DiskPath:       flags.diskPath,
				DiskBytes:      flags.diskBytes,
				DiskBlockBytes: flags.diskBlockBytes,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().DurationVar(&flags.duration, "duration", 0, "bounded pressure duration")
	command.Flags().IntVar(&flags.cpuWorkers, "cpu-workers", 0, "concurrent SHA-256 workers")
	command.Flags().Int64Var(&flags.memoryBytes, "memory-bytes", 0, "anonymous memory held and touched")
	command.Flags().StringVar(&flags.diskPath, "disk-path", "", "new absolute guest pressure file")
	command.Flags().Int64Var(&flags.diskBytes, "disk-bytes", 0, "maximum guest pressure file size")
	command.Flags().Int64Var(&flags.diskBlockBytes, "disk-block-bytes", 0, "synchronous write block size")
	return command
}

// pressureExecutablePath resolves the running acceptance binary copied into the guest.
func pressureExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve acceptance executable: %w", err)
	}
	return path, nil
}
