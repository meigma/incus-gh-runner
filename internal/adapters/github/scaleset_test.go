package github

import (
	"context"
	"errors"
	"testing"

	"github.com/actions/scaleset"
	"github.com/actions/scaleset/listener"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/incus-gh-runner/internal/controller"
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
					assert.Equal(t, "incus-phase4", name)
					return &scaleset.RunnerScaleSet{ID: 41, Name: name}, nil
				}
			},
			wantID: 41,
		},
		{
			name:        "creates a missing scale set in a named group",
			runnerGroup: "Build Runners",
			configureClient: func(client *fakeScaleSetClient) {
				client.getRunnerGroup = func(context.Context, string) (*scaleset.RunnerGroup, error) {
					return &scaleset.RunnerGroup{ID: 17, Name: "Build Runners"}, nil
				}
				client.getRunnerScaleSet = func(context.Context, int, string) (*scaleset.RunnerScaleSet, error) {
					return nil, nil //nolint:nilnil // A nil scale set is the upstream client's documented absent result.
				}
				client.createRunnerScaleSet = func(_ context.Context, requested *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error) {
					created := *requested
					client.created = &created
					return &scaleset.RunnerScaleSet{ID: 52, Name: requested.Name}, nil
				}
			},
			wantID: 52,
			wantCreated: &scaleset.RunnerScaleSet{
				Name:          "incus-phase4",
				RunnerGroupID: 17,
				Labels:        []scaleset.Label{{Name: "incus-phase4"}},
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
				Name:        "incus-phase4",
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

func TestScaleSetJITConfigUsesRunnerIdentityAndRejectsEmptyResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    *scaleset.RunnerScaleSetJitRunnerConfig
		responseErr error
		want        string
		wantErr     string
	}{
		{
			name:     "returns a fresh encoded configuration",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{EncodedJITConfig: "opaque-jit"},
			want:     "opaque-jit",
		},
		{
			name:     "rejects an empty configuration",
			response: &scaleset.RunnerScaleSetJitRunnerConfig{},
			wantErr:  "response is empty",
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

// fakeScaleSetClient provides behavior-controlled scale-set operations.
type fakeScaleSetClient struct {
	getRunnerGroup       func(context.Context, string) (*scaleset.RunnerGroup, error)
	getRunnerScaleSet    func(context.Context, int, string) (*scaleset.RunnerScaleSet, error)
	createRunnerScaleSet func(context.Context, *scaleset.RunnerScaleSet) (*scaleset.RunnerScaleSet, error)
	generateJIT          func(context.Context, *scaleset.RunnerScaleSetJitRunnerSetting, int) (*scaleset.RunnerScaleSetJitRunnerConfig, error)
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
