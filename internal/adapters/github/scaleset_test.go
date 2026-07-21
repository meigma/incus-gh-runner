package github

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
	"github.com/meigma/incus-gh-runner/internal/provenance"
)

func TestResolveScaleSetUsesExistingOrCreatesPersistentScaleSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runnerGroup     string
		configureClient func(*fakeScaleSetClient)
		wantID          int
		wantCreated     *scaleset.RunnerScaleSet
	}{
		{
			name:        "uses an existing scale set in the default group",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, groupID int, name string) (*scaleset.RunnerScaleSet, error) {
					assert.Equal(t, defaultRunnerGroupID, groupID)
					assert.Equal(t, "incus-runner-scale-set", name)
					return validRunnerScaleSetResponse(41, name, 0, "Default"), nil
				}
			},
			wantID: 41,
		},
		{
			name:        "creates a missing scale set in a named group",
			runnerGroup: "Build Runners",
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerGroup = func(_ context.Context, name string) (*scaleset.RunnerGroup, error) {
					assert.Equal(t, "Build Runners", name)
					return &scaleset.RunnerGroup{ID: 17, Name: "Build Runners"}, nil
				}
				client.getRunnerScaleSet = func(context.Context, int, string) (*scaleset.RunnerScaleSet, error) {
					return nil, nil //nolint:nilnil // A nil scale set is the upstream client's documented absent result.
				}
				client.createRunnerScaleSet = func(_ context.Context, requested *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
					created := *requested
					client.created = &created
					return validRunnerScaleSetResponse(52, requested.Name, 0, "Build Runners"), nil
				}
			},
			wantID: 52,
			wantCreated: &scaleset.RunnerScaleSet{
				Name:          "incus-runner-scale-set",
				RunnerGroupID: 17,
				Labels:        []scaleset.Label{{Name: "incus-runner-scale-set"}},
				RunnerSetting: scaleset.RunnerSetting{DisableUpdate: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeScaleSetClient()
			tt.configureClient(client)

			resolved, err := resolveScaleSet(context.Background(), client, ScaleSetOptions{
				Name:        "incus-runner-scale-set",
				RunnerGroup: tt.runnerGroup,
				SystemInfo:  scaleset.SystemInfo{System: "test"},
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, resolved.ID())
			assert.Equal(t, tt.wantCreated, client.created)
			assert.Equal(t, tt.wantID, client.systemInfo.ScaleSetID)
		})
	}
}

func TestResolveScaleSetRejectsMismatchedAPIIdentities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		runnerGroup     string
		configureClient func(*fakeScaleSetClient)
		wantErr         string
	}{
		{
			name:        "named lookup resolves the default group",
			runnerGroup: "Build Runners",
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerGroup = func(context.Context, string) (*scaleset.RunnerGroup, error) {
					return &scaleset.RunnerGroup{ID: defaultRunnerGroupID, Name: "Build Runners", IsDefault: true}, nil
				}
			},
			wantErr: "response identifies the default group",
		},
		{
			name:        "runner group response has another name",
			runnerGroup: "Build Runners",
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerGroup = func(context.Context, string) (*scaleset.RunnerGroup, error) {
					return &scaleset.RunnerGroup{ID: 17, Name: "Other Runners"}, nil
				}
			},
			wantErr: "response name does not match",
		},
		{
			name:        "runner group response has a negative ID",
			runnerGroup: "Build Runners",
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerGroup = func(context.Context, string) (*scaleset.RunnerGroup, error) {
					return &scaleset.RunnerGroup{ID: -1, Name: "Build Runners"}, nil
				}
			},
			wantErr: "response has no ID",
		},
		{
			name:        "scale set response has another name",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(context.Context, int, string) (*scaleset.RunnerScaleSet, error) {
					return validRunnerScaleSetResponse(
						41,
						"other-scale-set",
						defaultRunnerGroupID,
						"Default",
					), nil
				}
			},
			wantErr: "response name does not match",
		},
		{
			name:        "scale set response belongs to another group",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, _ int, name string) (*scaleset.RunnerScaleSet, error) {
					return validRunnerScaleSetResponse(41, name, 17, "Other Runners"), nil
				}
			},
			wantErr: "response runner group does not match",
		},
		{
			name:        "scale set response has a negative ID",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, _ int, name string) (*scaleset.RunnerScaleSet, error) {
					return validRunnerScaleSetResponse(-1, name, defaultRunnerGroupID, "Default"), nil
				}
			},
			wantErr: "response has no ID",
		},
		{
			name:        "scale set response names another group",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, _ int, name string) (*scaleset.RunnerScaleSet, error) {
					return validRunnerScaleSetResponse(41, name, defaultRunnerGroupID, "Other Runners"), nil
				}
			},
			wantErr: "response runner group name does not match",
		},
		{
			name:        "scale set response has an extra routing label",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, _ int, name string) (*scaleset.RunnerScaleSet, error) {
					response := validRunnerScaleSetResponse(41, name, defaultRunnerGroupID, "Default")
					response.Labels = append(response.Labels, scaleset.Label{Name: "generic-builder"})
					return response, nil
				}
			},
			wantErr: "response labels do not match",
		},
		{
			name:        "scale set response allows runner self-update",
			runnerGroup: scaleset.DefaultRunnerGroup,
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerScaleSet = func(_ context.Context, _ int, name string) (*scaleset.RunnerScaleSet, error) {
					response := validRunnerScaleSetResponse(41, name, defaultRunnerGroupID, "Default")
					response.RunnerSetting.DisableUpdate = false
					return response, nil
				}
			},
			wantErr: "does not disable runner self-update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeScaleSetClient()
			tt.configureClient(client)

			resolved, err := resolveScaleSet(context.Background(), client, ScaleSetOptions{
				Name:        "incus-runner-scale-set",
				RunnerGroup: tt.runnerGroup,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, resolved)
		})
	}
}

func validRunnerScaleSetResponse(
	id int,
	name string,
	runnerGroupID int,
	runnerGroupName string,
) *scaleset.RunnerScaleSet {
	return &scaleset.RunnerScaleSet{
		ID:              id,
		Name:            name,
		RunnerGroupID:   runnerGroupID,
		RunnerGroupName: runnerGroupName,
		Labels:          []scaleset.Label{{Name: name, Type: "System"}},
		RunnerSetting:   scaleset.RunnerSetting{DisableUpdate: true},
	}
}

func TestScaleSetJITConfigUsesRunnerIdentityAndRejectsEmptyResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    *scaleset.RunnerScaleSetJitRunnerConfig
		responseErr error
		want        JITConfig
		wantErr     string
	}{
		{
			name: "returns a validated runner reference with the opaque configuration",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{
				EncodedJITConfig: "opaque-jit",
				Runner: &scaleset.RunnerReference{
					ID: 41, Name: "runner-123", RunnerScaleSetID: 73,
				},
			},
			want: JITConfig{Encoded: "opaque-jit", RunnerID: 41, RunnerName: "runner-123", ScaleSetID: 73},
		},
		{
			name:     "rejects an empty configuration",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{},
			wantErr:  "response is incomplete",
		},
		{
			name: "rejects a mismatched runner name",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{
				EncodedJITConfig: "opaque-jit",
				Runner: &scaleset.RunnerReference{
					ID: 41, Name: "other-runner", RunnerScaleSetID: 73,
				},
			},
			wantErr: "runner response name does not match",
		},
		{
			name: "rejects a mismatched scale set",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{
				EncodedJITConfig: "opaque-jit",
				Runner: &scaleset.RunnerReference{
					ID: 41, Name: "runner-123", RunnerScaleSetID: 74,
				},
			},
			wantErr: "runner response scale set does not match",
		},
		{
			name:        "wraps an upstream failure",
			responseErr: errors.New("unavailable"),
			wantErr:     "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeScaleSetClient()
			client.generateJIT = func(
				_ context.Context,
				setting *scaleset.RunnerScaleSetJitRunnerSetting,
				scaleSetID int,
			) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
				assert.Equal(t, "runner-123", setting.Name)
				assert.Equal(t, defaultWorkFolder, setting.WorkFolder)
				assert.Equal(t, 73, scaleSetID)
				return tt.response, tt.responseErr
			}
			resolved := &ScaleSet{client: client, id: 73}

			got, err := resolved.JITConfig(context.Background(), "runner-123")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScaleSetFenceRemovesOnlyMatchingRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		lookups     []*scaleset.RunnerReference
		lookupErr   error
		removeErr   error
		wantRemoved []int64
		wantErr     string
	}{
		{name: "already absent", lookups: []*scaleset.RunnerReference{nil}},
		{
			name: "removes and confirms exact registration",
			lookups: []*scaleset.RunnerReference{
				{ID: 41, Name: "runner-123", RunnerScaleSetID: 73},
				nil,
			},
			wantRemoved: []int64{41},
		},
		{
			name:    "rejects another scale set",
			lookups: []*scaleset.RunnerReference{{ID: 41, Name: "runner-123", RunnerScaleSetID: 74}},
			wantErr: "does not match",
		},
		{
			name:      "propagates lookup failure",
			lookupErr: errors.New("inventory unavailable"),
			wantErr:   "inventory unavailable",
		},
		{
			name:        "propagates removal failure",
			lookups:     []*scaleset.RunnerReference{{ID: 41, Name: "runner-123", RunnerScaleSetID: 73}},
			removeErr:   errors.New("remove unavailable"),
			wantRemoved: []int64{41},
			wantErr:     "remove unavailable",
		},
		{
			name: "fails when registration remains",
			lookups: []*scaleset.RunnerReference{
				{ID: 41, Name: "runner-123", RunnerScaleSetID: 73},
				{ID: 41, Name: "runner-123", RunnerScaleSetID: 73},
			},
			wantRemoved: []int64{41},
			wantErr:     "still exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newFakeScaleSetClient()
			lookup := 0
			client.getRunnerByName = func(_ context.Context, runnerName string) (*scaleset.RunnerReference, error) {
				assert.Equal(t, "runner-123", runnerName)
				if tt.lookupErr != nil {
					return nil, tt.lookupErr
				}
				require.Less(t, lookup, len(tt.lookups))
				response := tt.lookups[lookup]
				lookup++
				return response, nil
			}
			var removed []int64
			client.removeRunner = func(_ context.Context, runnerID int64) error {
				removed = append(removed, runnerID)
				return tt.removeErr
			}

			err := (&ScaleSet{client: client, id: 73}).Fence(context.Background(), "runner-123")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRemoved, removed)
		})
	}
}

func TestDemandSourcePublishesAssignedJobsWithoutExternalWork(t *testing.T) {
	t.Parallel()

	upstream := &fakeDemandListener{run: func(ctx context.Context, scaler listener.Scaler) error {
		target, err := scaler.HandleDesiredRunnerCount(ctx, 9)
		require.NoError(t, err)
		assert.Equal(t, 4, target)
		started := &scaleset.JobStarted{
			RunnerName:     "runner-1",
			JobMessageBase: scaleset.JobMessageBase{JobID: "job-1"},
		}
		completed := &scaleset.JobCompleted{
			RunnerName:     "runner-1",
			Result:         "succeeded",
			JobMessageBase: scaleset.JobMessageBase{JobID: "job-1"},
		}
		require.NoError(t, scaler.HandleJobStarted(ctx, started))
		require.NoError(t, scaler.HandleJobCompleted(ctx, completed))
		return errors.New("poll stopped")
	}}
	source, err := newDemandSource(upstream, DemandSourceOptions{MinRunners: 1, MaxRunners: 4})
	require.NoError(t, err)
	mailbox := controller.NewMailbox()

	err = source.Run(context.Background(), mailbox.Publish)

	require.EqualError(t, err, "run scale-set listener: poll stopped")
	assert.Equal(t, controller.Demand{AssignedJobs: 9}, <-mailbox.Updates())
}

func TestDemandSourcePublishesAuthoritativeBusyRunnerSnapshot(t *testing.T) {
	t.Parallel()

	var observed []controller.Demand
	upstream := &fakeDemandListener{run: func(ctx context.Context, scaler listener.Scaler) error {
		require.NoError(t, scaler.HandleJobStarted(ctx, &scaleset.JobStarted{RunnerName: "runner-1"}))
		_, err := scaler.HandleDesiredRunnerCount(ctx, 1)
		require.NoError(t, err)
		require.NoError(t, scaler.HandleJobCompleted(ctx, &scaleset.JobCompleted{RunnerName: "runner-1"}))
		_, err = scaler.HandleDesiredRunnerCount(ctx, 0)
		require.NoError(t, err)
		return errors.New("poll stopped")
	}}
	source, err := newDemandSource(upstream, DemandSourceOptions{MaxRunners: 2})
	require.NoError(t, err)

	err = source.Run(context.Background(), func(demand controller.Demand) {
		observed = append(observed, demand)
	})

	require.EqualError(t, err, "run scale-set listener: poll stopped")
	assert.Equal(t, []controller.Demand{
		{AssignedJobs: 1, BusyRunners: []string{"runner-1"}},
		{BusyRunners: []string{}},
	}, observed)
}

// TestDemandHandlerQueuesValidatedJobProofEventsWithoutBlocking proves the synchronous callback boundary.
func TestDemandHandlerQueuesValidatedJobProofEventsWithoutBlocking(t *testing.T) {
	t.Parallel()

	queue, err := provenance.NewJobStartedQueue(1)
	require.NoError(t, err)
	handler := demandHandler{
		options: DemandSourceOptions{
			ScaleSetID:     73,
			ScaleSetName:   "incus-runners",
			JobStartedSink: queue,
			Logger:         slog.New(slog.DiscardHandler),
		},
		busy: make(map[string]struct{}),
	}
	job := &scaleset.JobStarted{
		RunnerID:   41,
		RunnerName: "runner-123",
		JobMessageBase: scaleset.JobMessageBase{
			OwnerName:       "meigma",
			RepositoryName:  "incus-gh-runner",
			JobWorkflowRef:  "meigma/incus-gh-runner/.github/workflows/test.yml@refs/heads/master",
			WorkflowRunID:   101,
			JobID:           "job-1",
			RunnerRequestID: 202,
			EventName:       "push",
		},
	}

	require.NoError(t, handler.HandleJobStarted(context.Background(), job))
	job.JobID = "job-2"
	require.NoError(
		t,
		handler.HandleJobStarted(context.Background(), job),
		"full proof queue must not fail the listener",
	)

	assert.Equal(t, provenance.JobStarted{
		Owner:           "meigma",
		Repository:      "incus-gh-runner",
		WorkflowRef:     "meigma/incus-gh-runner/.github/workflows/test.yml@refs/heads/master",
		WorkflowRunID:   101,
		JobID:           "job-1",
		RunnerRequestID: 202,
		RunnerID:        41,
		RunnerName:      "runner-123",
		EventName:       "push",
		ScaleSetID:      73,
		ScaleSetName:    "incus-runners",
	}, <-queue.Events())
	assert.Contains(t, handler.busy, "runner-123", "busy tracking must remain independent from queue saturation")
}

// TestDemandHandlerDropsMalformedProofEventWithoutFailingBusyTracking proves fail-closed event validation.
func TestDemandHandlerDropsMalformedProofEventWithoutFailingBusyTracking(t *testing.T) {
	t.Parallel()

	queue, err := provenance.NewJobStartedQueue(1)
	require.NoError(t, err)
	handler := demandHandler{
		options: DemandSourceOptions{
			ScaleSetID:     73,
			ScaleSetName:   "incus-runners",
			JobStartedSink: queue,
			Logger:         slog.New(slog.DiscardHandler),
		},
		busy: make(map[string]struct{}),
	}

	require.NoError(t, handler.HandleJobStarted(context.Background(), &scaleset.JobStarted{
		RunnerName: "runner-123",
	}))

	assert.Contains(t, handler.busy, "runner-123")
	select {
	case <-queue.Events():
		t.Fatal("malformed GitHub event must not enter the proof queue")
	default:
	}
}

func TestResilientDemandSourceCapsReconnectBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disconnected := newFakeMessageSession(func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
		return nil, errors.New("message queue unavailable")
	})
	recovered := newFakeMessageSession(
		func(ctx context.Context, _ int, _ int) (*scaleset.RunnerScaleSetMessage, error) {
			cancel()
			return nil, ctx.Err()
		},
	)
	attempts := 0
	open := func(context.Context) (messageSession, error) {
		attempts++
		switch attempts {
		case 1:
			return disconnected, nil
		case 2, 3:
			return nil, errors.New("session API unavailable")
		default:
			return recovered, nil
		}
	}
	var delays []time.Duration
	wait := func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	source, err := newResilientDemandSource(open, DemandSourceOptions{
		ScaleSetID:          73,
		MinRunners:          1,
		MaxRunners:          4,
		ReconnectInitial:    time.Second,
		ReconnectMaximum:    3 * time.Second,
		SessionCloseTimeout: time.Second,
	}, wait)
	require.NoError(t, err)

	err = source.Run(ctx, func(controller.Demand) {})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}, delays)
	assert.Equal(t, 1, disconnected.closeCount)
	assert.Equal(t, 1, recovered.closeCount)
}

func TestResilientDemandSourceResetsBackoffAfterHealthyPolling(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	initial := newFakeMessageSession(func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
		return nil, errors.New("message queue unavailable")
	})
	getCalls := 0
	healthy := newFakeMessageSession(func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error) {
		getCalls++
		if getCalls == 1 {
			return nil, nil //nolint:nilnil // A nil message is a healthy long-poll expiry.
		}
		return nil, errors.New("message queue disconnected")
	})
	attempts := 0
	open := func(context.Context) (messageSession, error) {
		attempts++
		switch attempts {
		case 1:
			return initial, nil
		case 2:
			return nil, errors.New("session API unavailable")
		default:
			return healthy, nil
		}
	}
	var delays []time.Duration
	wait := func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		if len(delays) == 3 {
			cancel()
			return ctx.Err()
		}
		return nil
	}
	source, err := newResilientDemandSource(open, DemandSourceOptions{
		ScaleSetID:          73,
		MinRunners:          1,
		MaxRunners:          4,
		ReconnectInitial:    time.Second,
		ReconnectMaximum:    10 * time.Second,
		SessionCloseTimeout: time.Second,
	}, wait)
	require.NoError(t, err)
	var generations []uint64

	err = source.Run(ctx, func(demand controller.Demand) {
		generations = append(generations, demand.Generation)
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, time.Second}, delays)
	assert.Equal(t, []uint64{1, 2, 2}, generations)
	assert.Equal(t, 1, initial.closeCount)
	assert.Equal(t, 1, healthy.closeCount)
}

func TestResilientDemandSourceRetriesInitialSessionOpen(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recovered := newFakeMessageSession(
		func(ctx context.Context, _ int, _ int) (*scaleset.RunnerScaleSetMessage, error) {
			cancel()
			return nil, ctx.Err()
		},
	)
	attempts := 0
	open := func(context.Context) (messageSession, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("active session conflict")
		}
		return recovered, nil
	}
	var delays []time.Duration
	wait := func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	source, err := newResilientDemandSource(open, DemandSourceOptions{
		ScaleSetID:          73,
		MinRunners:          1,
		MaxRunners:          4,
		ReconnectInitial:    time.Second,
		ReconnectMaximum:    10 * time.Second,
		SessionCloseTimeout: time.Second,
	}, wait)
	require.NoError(t, err)
	assert.Zero(t, attempts, "construction must not acquire a failure-prone external session")

	err = source.Run(ctx, func(controller.Demand) {})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, delays)
	assert.Equal(t, 1, recovered.closeCount)
}

// fakeScaleSetClient provides behavior-controlled scale-set operations.
type fakeScaleSetClient struct {
	getRunnerGroup       func(context.Context, string) (*scaleset.RunnerGroup, error)
	getRunnerScaleSet    func(context.Context, int, string) (*scaleset.RunnerScaleSet, error)
	createRunnerScaleSet func(context.Context, *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error)
	generateJIT          func(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
	getRunnerByName      func(context.Context, string) (*scaleset.RunnerReference, error)
	removeRunner         func(context.Context, int64) error
	created              *scaleset.RunnerScaleSet
	systemInfo           scaleset.SystemInfo
}

// newFakeScaleSetClient constructs a fake that fails unexpected calls.
func newFakeScaleSetClient() *fakeScaleSetClient {
	return &fakeScaleSetClient{
		getRunnerGroup: func(context.Context, string) (*scaleset.RunnerGroup, error) {
			return nil, errors.New("unexpected GetRunnerGroupByName call")
		},
		getRunnerScaleSet: func(context.Context, int, string) (*scaleset.RunnerScaleSet, error) {
			return nil, errors.New("unexpected GetRunnerScaleSet call")
		},
		createRunnerScaleSet: func(context.Context, *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
			return nil, errors.New("unexpected CreateRunnerScaleSet call")
		},
		generateJIT: func(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
			return nil, errors.New("unexpected GenerateJitRunnerConfig call")
		},
		getRunnerByName: func(context.Context, string) (*scaleset.RunnerReference, error) {
			return nil, errors.New("unexpected GetRunnerByName call")
		},
		removeRunner: func(context.Context, int64) error {
			return errors.New("unexpected RemoveRunner call")
		},
	}
}

// GetRunnerGroupByName invokes the configured runner-group behavior.
func (f *fakeScaleSetClient) GetRunnerGroupByName(ctx context.Context, name string) (*scaleset.RunnerGroup, error) {
	return f.getRunnerGroup(ctx, name)
}

// GetRunnerScaleSet invokes the configured scale-set lookup behavior.
func (f *fakeScaleSetClient) GetRunnerScaleSet(
	ctx context.Context,
	groupID int,
	name string,
) (*scaleset.RunnerScaleSet, error) {
	return f.getRunnerScaleSet(ctx, groupID, name)
}

// CreateRunnerScaleSet invokes the configured scale-set creation behavior.
func (f *fakeScaleSetClient) CreateRunnerScaleSet(
	ctx context.Context,
	scaleSet *scaleset.RunnerScaleSet,
) (*scaleset.RunnerScaleSet, error) {
	return f.createRunnerScaleSet(ctx, scaleSet)
}

// GenerateJitRunnerConfig invokes the configured JIT generation behavior.
func (f *fakeScaleSetClient) GenerateJitRunnerConfig(
	ctx context.Context,
	setting *scaleset.RunnerScaleSetJitRunnerSetting,
	scaleSetID int,
) (*scaleset.RunnerScaleSetJitRunnerConfig, error) {
	return f.generateJIT(ctx, setting, scaleSetID)
}

// GetRunnerByName invokes the configured runner lookup behavior.
func (f *fakeScaleSetClient) GetRunnerByName(
	ctx context.Context,
	runnerName string,
) (*scaleset.RunnerReference, error) {
	return f.getRunnerByName(ctx, runnerName)
}

// RemoveRunner invokes the configured runner-registration removal behavior.
func (f *fakeScaleSetClient) RemoveRunner(ctx context.Context, runnerID int64) error {
	return f.removeRunner(ctx, runnerID)
}

// SetSystemInfo records the configured scale-set identity.
func (f *fakeScaleSetClient) SetSystemInfo(info scaleset.SystemInfo) {
	f.systemInfo = info
}

// fakeDemandListener invokes one behavior-controlled listener run.
type fakeDemandListener struct {
	run func(context.Context, listener.Scaler) error
}

// Run invokes the configured message-loop behavior.
func (f *fakeDemandListener) Run(ctx context.Context, scaler listener.Scaler) error {
	return f.run(ctx, scaler)
}

// fakeMessageSession provides one behavior-controlled closeable listener client.
type fakeMessageSession struct {
	getMessage func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error)
	session    scaleset.RunnerScaleSetSession
	closeCount int
}

// newFakeMessageSession constructs a session with valid initial demand statistics.
func newFakeMessageSession(
	getMessage func(context.Context, int, int) (*scaleset.RunnerScaleSetMessage, error),
) *fakeMessageSession {
	return &fakeMessageSession{
		getMessage: getMessage,
		session: scaleset.RunnerScaleSetSession{
			SessionID:  uuid.New(),
			Statistics: &scaleset.RunnerScaleSetStatistic{},
		},
	}
}

// GetMessage invokes the configured message-poll behavior.
func (f *fakeMessageSession) GetMessage(
	ctx context.Context,
	lastMessageID int,
	maxCapacity int,
) (*scaleset.RunnerScaleSetMessage, error) {
	return f.getMessage(ctx, lastMessageID, maxCapacity)
}

// DeleteMessage accepts message acknowledgement in tests that produce messages.
func (f *fakeMessageSession) DeleteMessage(context.Context, int) error {
	return nil
}

// AcquireJobs accepts job acquisition in tests that produce available jobs.
func (f *fakeMessageSession) AcquireJobs(_ context.Context, requestIDs []int64) ([]int64, error) {
	return requestIDs, nil
}

// Session returns valid initial statistics for the upstream listener.
func (f *fakeMessageSession) Session() scaleset.RunnerScaleSetSession {
	return f.session
}

// Close records release of the fake message session.
func (f *fakeMessageSession) Close(context.Context) error {
	f.closeCount++
	return nil
}
