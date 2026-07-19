// Package incusvalidate compares a fail-closed Incus baseline with observed host state.
package incusvalidate

import "context"

// PolicyValidator validates a rendered baseline before any Incus API access occurs.
type PolicyValidator func(filename string, data []byte) error

// Reader returns the read-only Incus state needed by the validator.
type Reader interface {
	Read(ctx context.Context, names Names) (Snapshot, error)
}

// Result describes successful validation and any compatibility notices.
type Result struct {
	// Notices contains non-fatal security residuals operators must retain.
	Notices []string
}

// Baseline is the complete desired Incus state decoded from a CUE-validated manifest.
type Baseline struct {
	// SchemaVersion identifies the manifest contract.
	SchemaVersion int `json:"schema_version"`
	// Authority records the supported Incus trust boundary.
	Authority Authority `json:"authority"`
	// Names selects every Incus resource checked by the validator.
	Names Names `json:"names"`
	// Server declares required Incus daemon behavior and capabilities.
	Server ServerRequirements `json:"server"`
	// ResidualControls records compatibility controls that cannot yet be enforced directly.
	ResidualControls ResidualControls `json:"residual_controls"`
	// Project is the desired restricted runner project.
	Project Project `json:"project"`
	// Network is the desired host-owned runner bridge.
	Network Network `json:"network"`
	// NetworkACL is the desired host-owned runner ACL.
	NetworkACL NetworkACL `json:"network_acl"`
	// Profile is the desired sole runner profile.
	Profile Profile `json:"profile"`
	// StoragePool is the desired dedicated runner storage pool.
	StoragePool StoragePool `json:"storage_pool"`
}

// Authority describes the supported Incus connection authority.
type Authority struct {
	// Mode identifies the connection and authority model.
	Mode string `json:"mode"`
	// DedicatedSinglePurposeHostRequired records the dedicated-host requirement.
	DedicatedSinglePurposeHostRequired bool `json:"dedicated_single_purpose_host_required"`
	// UnixSocketIsRootEquivalent records the authority granted by the local socket.
	UnixSocketIsRootEquivalent bool `json:"unix_socket_is_root_equivalent"`
}

// Names contains the exact Incus resource names selected by a baseline.
type Names struct {
	// Project names the restricted runner project.
	Project string `json:"project"`
	// Network names the host-owned managed bridge.
	Network string `json:"network"`
	// NetworkACL names the host-owned default-deny ACL.
	NetworkACL string `json:"network_acl"`
	// Profile names the sole runner profile.
	Profile string `json:"profile"`
	// StoragePool names the dedicated runner storage pool.
	StoragePool string `json:"storage_pool"`
}

// ServerRequirements describes the Incus server properties required by a baseline.
type ServerRequirements struct {
	// MinimumVersion is the oldest accepted Incus server version.
	MinimumVersion string `json:"minimum_version"`
	// RequiredAPIExtensions lists every API capability required by the baseline.
	RequiredAPIExtensions []string `json:"required_api_extensions"`
	// FirewallDriver is the required host firewall implementation.
	FirewallDriver string `json:"firewall_driver"`
	// Standalone requires a non-clustered Incus daemon.
	Standalone bool `json:"standalone"`
	// CoreHTTPSAddress is the required public API listener value.
	CoreHTTPSAddress string `json:"core_https_address"`
	// ClusterHTTPSAddress is the required cluster API listener value.
	ClusterHTTPSAddress string `json:"cluster_https_address"`
}

// ResidualControls contains compatibility controls awaiting direct Incus support.
type ResidualControls struct {
	// ProjectVMNestingRestriction describes the VM nesting compatibility path.
	ProjectVMNestingRestriction NestingRestriction `json:"project_vm_nesting_restriction"`
}

// NestingRestriction describes the temporary profile-level VM nesting control.
type NestingRestriction struct {
	// Status explains why project-level enforcement is unavailable.
	Status string `json:"status"`
	// FutureAPIExtension names the extension that will supersede the compatibility path.
	FutureAPIExtension string `json:"future_api_extension"`
	// CompensatingProfileKey names the profile key enforcing the current control.
	CompensatingProfileKey string `json:"compensating_profile_key"`
	// CompensatingProfileValue is the required profile value for the current control.
	CompensatingProfileValue string `json:"compensating_profile_value"`
}

// Project contains the writable project state compared by the validator.
type Project struct {
	// Description explains the project's dedicated purpose.
	Description string `json:"description"`
	// Config contains the exact project features, limits, and restrictions.
	Config map[string]string `json:"config"`
}

// Network contains the managed network state compared by the validator.
type Network struct {
	// Description explains the network's dedicated purpose.
	Description string `json:"description"`
	// Type identifies the Incus network driver family.
	Type string `json:"type"`
	// Managed records whether Incus manages the network.
	Managed bool `json:"managed"`
	// Config contains the exact network settings.
	Config map[string]string `json:"config"`
}

// NetworkACL contains the writable ACL state compared by the validator.
type NetworkACL struct {
	// Description explains the ACL's controlled-egress purpose.
	Description string `json:"description"`
	// Config contains ACL-level settings.
	Config map[string]string `json:"config"`
	// Ingress contains explicit ingress rules.
	Ingress []NetworkACLRule `json:"ingress"`
	// Egress contains explicit egress rules.
	Egress []NetworkACLRule `json:"egress"`
}

// NetworkACLRule contains every writable Incus ACL rule field.
type NetworkACLRule struct {
	// Action selects the behavior applied on a match.
	Action string `json:"action"`
	// Source restricts matching source addresses.
	Source string `json:"source,omitempty"`
	// Destination restricts matching destination addresses.
	Destination string `json:"destination,omitempty"`
	// Protocol restricts the network protocol.
	Protocol string `json:"protocol,omitempty"`
	// SourcePort restricts matching source ports.
	SourcePort string `json:"source_port,omitempty"`
	// DestinationPort restricts matching destination ports.
	DestinationPort string `json:"destination_port,omitempty"`
	// ICMPType restricts matching ICMP message types.
	ICMPType string `json:"icmp_type,omitempty"`
	// ICMPCode restricts matching ICMP message codes.
	ICMPCode string `json:"icmp_code,omitempty"`
	// Description explains the rule's purpose.
	Description string `json:"description,omitempty"`
	// State enables or disables the rule.
	State string `json:"state"`
}

// Profile contains the writable profile state compared by the validator.
type Profile struct {
	// Description explains the profile's exclusive runner purpose.
	Description string `json:"description"`
	// Config contains the exact VM and security settings.
	Config map[string]string `json:"config"`
	// Devices contains the exact devices applied to runner VMs.
	Devices map[string]map[string]string `json:"devices"`
}

// StoragePool contains the storage-pool state compared by the validator.
type StoragePool struct {
	// Description explains the pool's dedicated runner purpose.
	Description string `json:"description"`
	// Driver identifies the storage implementation.
	Driver string `json:"driver"`
	// Config contains the exact storage-pool settings.
	Config map[string]string `json:"config"`
}

// ServerState contains the observed daemon properties needed for validation.
type ServerState struct {
	// Auth reports whether the local client is trusted.
	Auth string
	// APIExtensions lists the capabilities advertised by the daemon.
	APIExtensions []string
	// Config contains daemon configuration relevant to network exposure.
	Config map[string]string
	// Version is the running Incus server version.
	Version string
	// Clustered reports whether the daemon belongs to a cluster.
	Clustered bool
	// FirewallDriver identifies the active firewall implementation.
	FirewallDriver string
}

// Snapshot contains the complete observed state read from Incus.
type Snapshot struct {
	// Server contains daemon state and capabilities.
	Server ServerState
	// Project contains the selected runner project.
	Project Project
	// Network contains the selected default-project network.
	Network Network
	// NetworkACL contains the selected default-project ACL.
	NetworkACL NetworkACL
	// Profile contains the selected runner-project profile.
	Profile Profile
	// StoragePool contains the selected global storage pool.
	StoragePool StoragePool
}
