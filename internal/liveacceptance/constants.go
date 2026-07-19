package liveacceptance

import "time"

const (
	acceptanceRunnerCount = 2
	localNameMaxLength    = 63
	trueValue             = "true"
	falseValue            = "false"
	failedValue           = "failed"
	readyValue            = "ready"
	incusExecArgument     = "exec"
	listArgument          = "list"
	curlCommand           = "curl"
	curlSilentArgument    = "--silent"
	curlShowErrorArgument = "--show-error"
	curlMaxTimeArgument   = "--max-time"

	shortCommandTimeout    = 10 * time.Second
	stateCommandTimeout    = 15 * time.Second
	listenerCommandTimeout = 3 * time.Second
	commandTimeout         = 30 * time.Second
	guestEgressTimeout     = 35 * time.Second
	mutationTimeout        = time.Minute
	transferTimeout        = 2 * time.Minute
	admissionTimeout       = 2 * time.Minute
	probeOverheadTimeout   = 10 * time.Minute

	defaultStressDuration    = 10 * time.Minute
	maximumStressDuration    = 15 * time.Minute
	minimumStressDuration    = 10 * time.Minute
	defaultPollInterval      = time.Second
	minimumPollInterval      = 100 * time.Millisecond
	maximumPollInterval      = 10 * time.Second
	ipv6ListenerPollInterval = 250 * time.Millisecond

	gibibyte                  uint64 = 1 << 30
	mebibyte                  uint64 = 1 << 20
	megabitPerSecond          uint64 = 1_000_000
	kibibyte                  uint64 = 1024
	canaryRandomBytes                = 32
	maximumReasonBytes               = 500
	maximumCommandErrorBytes         = 300
	maximumCommandStdoutBytes        = 8 << 20
	maximumCommandStderrBytes        = 1 << 20
	pressureCPUFactor                = 4
	rootPressureDiskDivisor          = 20
	minimumMaterialDiskBytes         = 64 << 20
	egressSamplePeriod               = 10
	rootSizeFieldCount               = 2
	propertyFieldCount               = 2
)
