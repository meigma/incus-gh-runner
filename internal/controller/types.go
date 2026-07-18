// Package controller reconciles GitHub runner demand with owned runner capacity.
package controller

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrShutdownTimeout reports that a backend operation ignored cancellation
// beyond the controller's bounded shutdown window.
var ErrShutdownTimeout = errors.New("controller shutdown timed out")

// RunnerState describes capacity that the controller owns.
type RunnerState string

const (
	// RunnerProvisioning is an owned runner that is not ready yet.
	RunnerProvisioning RunnerState = "provisioning"
	// RunnerIdle is an owned runner that can accept a job.
	RunnerIdle RunnerState = "idle"
	// RunnerBusy is an owned runner that is executing a job.
	RunnerBusy RunnerState = "busy"
	// RunnerTerminal is an owned runner that should be deleted.
	RunnerTerminal RunnerState = "terminal"
)

// Runner describes one controller-owned unit of capacity.
type Runner struct {
	// ID is the backend's stable, log-safe runner identifier.
	ID string
	// State is the runner's currently observed lifecycle state.
	State RunnerState
}

// Demand contains the latest scale-set demand relevant to capacity.
type Demand struct {
	// AssignedJobs is the number of jobs currently assigned to the scale set.
	AssignedJobs int
}

// Backend performs external runner inventory and lifecycle operations.
type Backend interface {
	// ListOwned returns only runners carrying this controller's ownership marker.
	ListOwned(ctx context.Context) ([]Runner, error)
	// Create creates one owned runner and returns its observed state.
	Create(ctx context.Context) (Runner, error)
	// Delete removes the identified owned runner.
	Delete(ctx context.Context, runnerID string) error
}

// Options configures a Controller.
type Options struct {
	// Backend performs runner inventory and lifecycle operations.
	Backend Backend
	// Demand supplies coalesced scale-set demand updates.
	Demand <-chan Demand
	// Logger receives structured, secret-safe lifecycle events.
	Logger *slog.Logger
	// MinRunners is the idle capacity floor.
	MinRunners int
	// MaxRunners is the hard capacity ceiling.
	MaxRunners int
	// Workers bounds concurrent backend lifecycle operations.
	Workers int
	// ReconcileInterval controls the periodic safety reconciliation tick.
	ReconcileInterval time.Duration
	// OperationTimeout bounds each backend lifecycle operation.
	OperationTimeout time.Duration
	// ShutdownTimeout allows in-flight operations to finish before cancellation.
	ShutdownTimeout time.Duration
}

// Mailbox keeps only the newest demand update without blocking its publisher.
type Mailbox struct {
	mu      sync.Mutex
	updates chan Demand
}

// NewMailbox creates a demand mailbox.
func NewMailbox() *Mailbox {
	return &Mailbox{updates: make(chan Demand, 1)}
}

// Publish replaces any pending update with demand.
func (m *Mailbox) Publish(demand Demand) {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.updates:
	default:
	}

	m.updates <- demand
}

// Updates returns the mailbox's stream of coalesced demand.
func (m *Mailbox) Updates() <-chan Demand {
	return m.updates
}
