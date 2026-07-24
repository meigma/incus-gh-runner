// Package incusrunner renders a hardened Incus configuration for ephemeral
// GitHub Actions runner virtual machines.
package incusrunner

import (
	"net"
	"strconv"
	"strings"
)

_#Name: (string & =~"^[a-z][a-z0-9-]{0,62}$") |
			error("name must be a lowercase Incus identifier of at most 63 characters")
_#DedicatedName: (_#Name & !="default") |
		error("dedicated Incus resource name must not be default")
_#BridgeName: (_#DedicatedName & =~"^[a-z][a-z0-9-]{1,14}$") |
		error("managed bridge name must be 2 to 15 characters to fit the Linux interface limit")
_#PositiveInt: (int & >=1) | error("value must be a positive integer")
_#ProxyPort:   (int & >=1 & <=65535 & !=53) |
			error("proxy port must be between 1 and 65535 and must not be the DNS port")
_#EndpointPort: (int & >=1 & <=65535) |
	error("endpoint port must be between 1 and 65535")
_#IPv4:          (net.IP & !~":") | error("value must be an IPv4 address")
_#IPv4CIDR:      (net.IPCIDR & !~":") | error("value must be an IPv4 CIDR")
_#StorageSource: (string & =~"^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,254}$") |
		error("storage source contains unsupported characters")
_#StorageName: (string & =~"^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$") |
	error("storage name contains unsupported characters")

_#ZFSStorageInput: {
	driver:  *"zfs" | "zfs"
	source!: _#StorageSource
}

_#LVMStorageInput: {
	driver:         "lvm"
	source!:        _#StorageName
	thinPoolName!:  _#StorageName
	volumeSizeGiB!: int & >=1 & <=16384
}

// #Inputs is the complete operator-controlled configuration surface. Fields
// absent from this definition are deliberately not configurable.
#Inputs: {
	// names groups the dedicated Incus resource names used by the deployment.
	names: {
		// project names the restricted project that owns runner VMs and profiles.
		project: _#DedicatedName & (*"github-runners" | string)
		// network names the host-owned managed bridge and fits the Linux interface-name limit.
		network: _#BridgeName & (*"runner-network" | string)
		// networkACL names the host-owned default-deny network ACL.
		networkACL: _#DedicatedName & (*"runner-egress" | string)
		// profile names the sole profile applied to each runner VM.
		profile: _#DedicatedName & (*"github-runner" | string)
		// storagePool names the dedicated storage pool available to the project.
		storagePool: _#DedicatedName & (*"runner-storage" | string)
	}

	// host declares usable physical capacity and mandatory non-runner headroom.
	host: {
		// cpu is the number of logical host CPUs available to Incus and the host.
		cpu!: _#PositiveInt
		// memoryGiB is the usable host memory in GiB.
		memoryGiB!: _#PositiveInt
		// storageGiB is the usable storage capacity in GiB.
		storageGiB!: _#PositiveInt
		// reserve keeps capacity available for Incus, the controller, and the host.
		reserve: {
			// cpu is the number of host CPUs excluded from runner allocation.
			cpu: _#PositiveInt & (*4 | int)
			// memoryGiB is the host memory in GiB excluded from runner allocation.
			memoryGiB: _#PositiveInt & (*8 | int)
			// storageGiB is the host storage in GiB excluded from runner allocation.
			storageGiB: _#PositiveInt & (*40 | int)
		}
	}

	// runners controls concurrency, per-VM limits, and shared image capacity.
	runners: {
		// maximum is the maximum concurrent runner count and project VM ceiling.
		maximum: (int & >=1 & <=100) & (*10 | int)
		// cpu is the logical CPU limit applied to each runner VM.
		cpu: (int & >=1 & <=64) & (*2 | int)
		// memoryGiB is the memory limit in GiB applied to each runner VM.
		memoryGiB: (int & >=1 & <=1024) & (*4 | int)
		// rootDiskGiB is the root disk size in GiB allocated to each runner VM.
		rootDiskGiB: (int & >=1 & <=16384) & (*20 | int)
		// networkMbit is the maximum network throughput in Mbit per runner VM.
		networkMbit: (int & >=1 & <=100000) & (*100 | int)
		// diskIOMiB is the requested maximum disk throughput in MiB per runner VM.
		diskIOMiB: (int & >=1 & <=100000) & (*100 | int)
		// imageCacheGiB reserves storage in GiB for project image caching.
		imageCacheGiB: (int & >=1 & <=16384) & (*20 | int)
	}

	// network configures the isolated IPv4 bridge and controlled egress endpoints.
	network: {
		// ipv4Address is the managed bridge address and prefix in IPv4 CIDR form.
		ipv4Address!: _#IPv4CIDR
		// dnsAddress is the dedicated IPv4 resolver runners may contact.
		dnsAddress!: _#IPv4
		// proxy configures the HTTP CONNECT proxy runners may contact.
		proxy: {
			// address is the dedicated proxy IPv4 address.
			address!: _#IPv4
			// port is the proxy TCP port and must not overlap DNS port 53.
			port: _#ProxyPort & (*3128 | int)
		}
		// additionalEgress appends narrowly scoped endpoints to the fixed DNS and proxy policy.
		additionalEgress: *[] | [...{
			// name identifies the endpoint in the rendered ACL description.
			name!: _#Name
			// address is the endpoint's exact IPv4 destination.
			address!: _#IPv4
			// protocol restricts the endpoint to TCP or UDP.
			protocol!: "tcp" | "udp"
			// port is the endpoint's single destination port.
			port!: _#EndpointPort
		}]
	}

	// storage selects one narrowly configured dedicated backing store.
	storage: *_#ZFSStorageInput | _#LVMStorageInput
}

_#PositiveDecimalString: string & =~"^[1-9][0-9]*$"
_#PositiveGiBString:     string & =~"^[1-9][0-9]*GiB$"
_#PositiveMiBString:     string & =~"^[1-9][0-9]*MiB$"
_#PositiveMbitString:    string & =~"^[1-9][0-9]*Mbit$"
_#IPv4HostCIDR:          _#IPv4CIDR & =~"/32$"

// _#Baseline is the closed runtime-validation schema for a rendered Incus
// baseline. It owns every fixed security policy value and validates the
// relationships among the operator-derived values serialized in the manifest.
_#Baseline: {
	_maximum: strconv.Atoi(project.config["limits.instances"])
	_maximum: >=1 & <=100

	_runnerCPU:  strconv.Atoi(profile.config["limits.cpu"])
	_runnerCPU:  >=1 & <=64
	_projectCPU: strconv.Atoi(project.config["limits.cpu"])
	_projectCPU: _maximum * _runnerCPU

	_runnerMemoryGiB:  strconv.Atoi(strings.TrimSuffix(profile.config["limits.memory"], "GiB"))
	_runnerMemoryGiB:  >=1 & <=1024
	_projectMemoryGiB: strconv.Atoi(strings.TrimSuffix(project.config["limits.memory"], "GiB"))
	_projectMemoryGiB: _maximum * _runnerMemoryGiB

	_runnerRootDiskGiB: strconv.Atoi(strings.TrimSuffix(profile.devices.root.size, "GiB"))
	_runnerRootDiskGiB: >=1 & <=16384
	_projectDiskGiB:    strconv.Atoi(strings.TrimSuffix(project.config["limits.disk"], "GiB"))
	_imageCacheGiB:     _projectDiskGiB - _maximum*_runnerRootDiskGiB
	_imageCacheGiB:     >=1 & <=16384

	_networkMbit: strconv.Atoi(strings.TrimSuffix(profile.devices.eth0["limits.max"], "Mbit"))
	_networkMbit: >=1 & <=100000
	_diskIOMiB:   strconv.Atoi(strings.TrimSuffix(profile.devices.root["limits.max"], "MiB"))
	_diskIOMiB:   >=1 & <=100000
	_proxyPort:   strconv.Atoi(network_acl.egress[2].destination_port)
	_proxyPort:   >=1 & <=65535 & !=53
	_additionalEgressCount: len(network_acl.egress) - 3
	_additionalEgressCount: >=0 & <=16

	// schema_version identifies the baseline manifest schema.
	schema_version: 1
	// authority documents the supported Incus control-plane trust boundary.
	authority: {
		// mode identifies the controller's Incus connection and authority model.
		mode: "dedicated-host-unix-socket"
		// dedicated_single_purpose_host_required requires workload isolation at the host boundary.
		dedicated_single_purpose_host_required: true
		// unix_socket_is_root_equivalent records the authority granted by the Incus Unix socket.
		unix_socket_is_root_equivalent: true
	}
	// names records the exact Incus resources selected by this baseline.
	names: {
		// project names the restricted project that owns runner VMs and profiles.
		project: _#DedicatedName
		// network names the host-owned managed bridge and fits the Linux interface-name limit.
		network: _#BridgeName
		// network_acl names the host-owned default-deny network ACL.
		network_acl: _#DedicatedName
		// profile names the sole profile applied to each runner VM.
		profile: _#DedicatedName
		// storage_pool names the dedicated storage pool available to the project.
		storage_pool: _#DedicatedName
	}
	// server constrains the Incus daemon features and exposure model.
	server: {
		// minimum_version is the oldest supported Incus server release.
		minimum_version: "7.0"
		// required_api_extensions lists every API capability required by the baseline.
		required_api_extensions: [
			"container_nic_ipfilter",
			"instance_nic_bridged_port_isolation",
			"network_acl",
			"network_bridge_acl",
			"network_bridge_acl_devices",
			"projects_limits_disk_pool",
			"projects_networks",
			"projects_networks_restricted_access",
			"projects_restricted_storage_pool_access",
			"projects_restrictions",
		]
		// firewall_driver is the required host firewall implementation.
		firewall_driver: "nftables"
		// standalone requires a non-clustered Incus server.
		standalone: true
		// core_https_address requires the public Incus HTTPS listener to be disabled.
		core_https_address: ""
		// cluster_https_address requires the Incus cluster listener to be disabled.
		cluster_https_address: ""
	}
	// residual_controls documents controls that require compatibility handling.
	residual_controls: {
		// project_vm_nesting_restriction records the current VM nesting residual.
		project_vm_nesting_restriction: {
			// status describes why the project-level control is not emitted.
			status: "unsupported-by-incus-7.0-through-7.2"
			// future_api_extension names the extension that will make the control available.
			future_api_extension: "projects_restricted_virtual_machines_nesting"
			// compensating_profile_key names the profile setting used in the interim.
			compensating_profile_key: "security.nesting"
			// compensating_profile_value is the required interim setting value.
			compensating_profile_value: "false"
		}
	}
	// project is the desired restricted Incus project configuration.
	project: {
		// description explains the project's dedicated runner purpose.
		description: "Restricted project for ephemeral GitHub runner VMs"
		// config contains the exact project features, limits, and restrictions.
		config: {
			// `features.images` enables project-scoped cached images.
			"features.images": "true"
			// `features.networks` prevents the project from owning managed networks.
			"features.networks": "false"
			// `features.profiles` enables the project-local runner profile.
			"features.profiles": "true"
			// `features.storage.buckets` enables project scoping for storage buckets.
			"features.storage.buckets": "true"
			// `features.storage.volumes` enables project scoping for custom volumes.
			"features.storage.volumes": "true"
			// `images.auto_update_cached` disables automatic cached-image updates.
			"images.auto_update_cached": "false"
			// `images.auto_update_interval` disables periodic image refreshes.
			"images.auto_update_interval": "0"
			// `limits.containers` prevents container creation in the VM-only project.
			"limits.containers": "0"
			// `limits.cpu` is the aggregate logical CPU ceiling for runner VMs.
			"limits.cpu": _#PositiveDecimalString
			// `limits.disk` is the aggregate disk ceiling for runner VMs and images.
			"limits.disk": _#PositiveGiBString
			// `limits.disk.pool.<storage-pool>` applies the disk ceiling to the dedicated pool.
			("limits.disk.pool." + names.storage_pool): project.config["limits.disk"]
			// `limits.instances` is the aggregate runner instance ceiling.
			"limits.instances": _#PositiveDecimalString
			// `limits.memory` is the aggregate memory ceiling for runner VMs.
			"limits.memory": _#PositiveGiBString
			// `limits.networks` prevents project-owned network creation.
			"limits.networks": "0"
			// `limits.virtual-machines` is the aggregate runner VM ceiling.
			"limits.virtual-machines": project.config["limits.instances"]
			// `restricted` enables Incus project restrictions.
			"restricted": "true"
			// `restricted.backups` blocks project backup operations.
			"restricted.backups": "block"
			// `restricted.cluster.target` blocks explicit cluster placement.
			"restricted.cluster.target": "block"
			// `restricted.containers.interception` blocks syscall interception controls.
			"restricted.containers.interception": "block"
			// `restricted.containers.lowlevel` blocks low-level container configuration.
			"restricted.containers.lowlevel": "block"
			// `restricted.containers.nesting` blocks nested container workloads.
			"restricted.containers.nesting": "block"
			// `restricted.containers.privilege` permits only unprivileged containers.
			"restricted.containers.privilege": "unprivileged"
			// `restricted.devices.disk` blocks unrestricted disk devices.
			"restricted.devices.disk": "block"
			// `restricted.devices.gpu` blocks GPU passthrough devices.
			"restricted.devices.gpu": "block"
			// `restricted.devices.infiniband` blocks InfiniBand passthrough devices.
			"restricted.devices.infiniband": "block"
			// `restricted.devices.nic` permits only managed network interfaces.
			"restricted.devices.nic": "managed"
			// `restricted.devices.pci` blocks PCI passthrough devices.
			"restricted.devices.pci": "block"
			// `restricted.devices.proxy` blocks Incus proxy devices.
			"restricted.devices.proxy": "block"
			// `restricted.devices.unix-block` blocks Unix block devices.
			"restricted.devices.unix-block": "block"
			// `restricted.devices.unix-char` blocks Unix character devices.
			"restricted.devices.unix-char": "block"
			// `restricted.devices.unix-hotplug` blocks Unix hotplug devices.
			"restricted.devices.unix-hotplug": "block"
			// `restricted.devices.usb` blocks USB passthrough devices.
			"restricted.devices.usb": "block"
			// `restricted.networks.access` allowlists the host-owned runner bridge.
			"restricted.networks.access": names.network
			// `restricted.snapshots` blocks project snapshot operations.
			"restricted.snapshots": "block"
			// `restricted.storage-pools.access` allowlists the dedicated runner pool.
			"restricted.storage-pools.access": names.storage_pool
			// `restricted.virtual-machines.lowlevel` blocks low-level VM configuration.
			"restricted.virtual-machines.lowlevel": "block"
		}
	}
	// network is the desired host-owned managed bridge configuration.
	network: {
		// description explains the bridge's isolated runner purpose.
		description: "Managed bridge for isolated ephemeral runner VMs"
		// type selects an Incus bridge network.
		type: "bridge"
		// managed requires Incus to manage the bridge configuration.
		managed: true
		// config contains the exact bridge, DNS, routing, and ACL settings.
		config: {
			// `bridge.driver` selects the native Linux bridge driver.
			"bridge.driver": "native"
			// `dns.mode` disables Incus DNS records for runner instances.
			"dns.mode": "none"
			// `dns.nameservers` advertises only the controlled resolver.
			"dns.nameservers": _#IPv4
			// `ipv4.address` assigns the configured IPv4 bridge subnet.
			"ipv4.address": _#IPv4CIDR
			// `ipv4.dhcp` enables address assignment on the isolated bridge.
			"ipv4.dhcp": "true"
			// `ipv4.firewall` enables Incus firewall rules for the bridge.
			"ipv4.firewall": "true"
			// `ipv4.nat` enables IPv4 source NAT after ACL enforcement.
			"ipv4.nat": "true"
			// `ipv4.routing` enables IPv4 forwarding for controlled egress.
			"ipv4.routing": "true"
			// `ipv6.address` disables IPv6 address assignment and routing.
			"ipv6.address": "none"
			// `raw.dnsmasq` disables the bridge host's DNS forwarding service.
			"raw.dnsmasq": "port=0"
			// `security.acls` attaches the default-deny runner ACL to the bridge.
			"security.acls": names.network_acl
			// `security.acls.default.egress.action` rejects unmatched egress.
			"security.acls.default.egress.action": "reject"
			// `security.acls.default.egress.logged` logs rejected unmatched egress.
			"security.acls.default.egress.logged": "true"
			// `security.acls.default.ingress.action` rejects unmatched ingress.
			"security.acls.default.ingress.action": "reject"
			// `security.acls.default.ingress.logged` logs rejected unmatched ingress.
			"security.acls.default.ingress.logged": "true"
		}
	}
	// network_acl is the desired controlled-egress ACL configuration.
	network_acl: {
		// description explains the ACL's controlled-egress purpose.
		description: "Default-deny runner egress through controlled DNS and HTTPS proxy" |
			"Default-deny runner egress through controlled endpoints"
		// config contains ACL-level settings; none are permitted by this baseline.
		config: {}
		// ingress contains explicit ingress permits; none are permitted.
		ingress: []
		egress: [
			{destination: "\(network.config["dns.nameservers"])/32"},
			{destination: "\(network.config["dns.nameservers"])/32"},
			...,
		]
		// egress contains fixed DNS and proxy rules followed by at most 16 exact endpoints.
		egress: [_#DNSUDPRule, _#DNSTCPRule, _#ProxyRule, ..._#AdditionalEgressRule]
	}
	// profile is the desired sole runner VM profile configuration.
	profile: {
		// description explains the profile's exclusive runner purpose.
		description: "Only profile applied to ephemeral GitHub runner VMs"
		// config contains the exact VM boot, resource, and security settings.
		config: {
			// `boot.autostart` prevents runner VMs from starting with the host.
			"boot.autostart": "false"
			// `limits.cpu` applies the configured per-runner logical CPU ceiling.
			"limits.cpu": _#PositiveDecimalString
			// `limits.memory` applies the configured per-runner memory ceiling.
			"limits.memory": _#PositiveGiBString
			// `security.guestapi` prevents guest access to the Incus guest API.
			"security.guestapi": "false"
			// `security.nesting` prevents nested workloads inside runner VMs.
			"security.nesting": "false"
			// `security.secureboot` requires UEFI Secure Boot for runner VMs.
			"security.secureboot": "true"
		}
		// devices contains the only NIC and root disk applied to runner VMs.
		devices: {
			// eth0 is the isolated managed network interface.
			eth0: {
				// type selects an Incus network interface device.
				type: "nic"
				// network attaches the interface to the host-owned runner bridge.
				network: names.network
				// `limits.max` applies the configured per-runner network ceiling.
				"limits.max": _#PositiveMbitString
				// `ipv6.address` rejects all guest IPv6 traffic when IPv6 filtering is enabled.
				"ipv6.address"!: "none"
				// `security.acls` attaches the default-deny runner ACL to the interface.
				"security.acls": names.network_acl
				// `security.acls.default.egress.action` rejects unmatched NIC egress.
				"security.acls.default.egress.action": "reject"
				// `security.acls.default.egress.logged` logs rejected NIC egress.
				"security.acls.default.egress.logged": "true"
				// `security.acls.default.ingress.action` rejects unmatched NIC ingress.
				"security.acls.default.ingress.action": "reject"
				// `security.acls.default.ingress.logged` logs rejected NIC ingress.
				"security.acls.default.ingress.logged": "true"
				// `security.ipv4_filtering` blocks IPv4 address spoofing.
				"security.ipv4_filtering": "true"
				// `security.ipv6_filtering` blocks IPv6 address spoofing.
				"security.ipv6_filtering": "true"
				// `security.mac_filtering` blocks MAC address spoofing.
				"security.mac_filtering": "true"
				// `security.port_isolation` blocks peer traffic between runner ports.
				"security.port_isolation": "true"
			}
			// root is the runner VM root disk.
			root: {
				// type selects an Incus disk device.
				type: "disk"
				// path mounts this disk as the guest root filesystem.
				path: "/"
				// pool stores the root disk on the dedicated runner pool.
				pool: names.storage_pool
				// size applies the configured per-runner root disk allocation.
				size: _#PositiveGiBString
				// `limits.max` requests the configured per-runner disk throughput ceiling.
				"limits.max": _#PositiveMiBString
			}
		}
	}
	// storage_pool is the desired dedicated runner storage pool configuration.
	storage_pool: _#ZFSStoragePool | _#LVMStoragePool
}

_#DNSUDPRule: {
	action:           "allow"
	state:            "enabled"
	description:      "Controlled DNS over UDP"
	destination:      _#IPv4HostCIDR
	protocol:         "udp"
	destination_port: "53"
}

_#DNSTCPRule: {
	action:           "allow"
	state:            "enabled"
	description:      "Controlled DNS over TCP"
	destination:      _#IPv4HostCIDR
	protocol:         "tcp"
	destination_port: "53"
}

_#ProxyRule: {
	action:           "allow"
	state:            "enabled"
	description:      "Controlled HTTP CONNECT proxy"
	destination:      _#IPv4HostCIDR
	protocol:         "tcp"
	destination_port: _#PositiveDecimalString
}

_#AdditionalEgressRule: {
	action:           "allow"
	state:            "enabled"
	description:      string & =~"^Controlled additional egress: [a-z][a-z0-9-]{0,62}$"
	destination:      _#IPv4HostCIDR
	protocol:         "tcp" | "udp"
	destination_port: _#PositiveDecimalString
}

// _#ZFSStoragePool is the exact supported ZFS pool state.
_#ZFSStoragePool: {
	description: "Dedicated ZFS pool for ephemeral runner images and VM roots"
	driver:      "zfs"
	config: {
		source:          _#StorageSource
		"zfs.pool_name": source
	}
}

// _#LVMStoragePool is the exact supported LVM thin-pool state.
_#LVMStoragePool: {
	description: "Dedicated LVM thin pool for ephemeral runner images and VM roots"
	driver:      "lvm"
	config: {
		source:              _#StorageName
		"lvm.vg_name":       source
		"lvm.thinpool_name": _#StorageName
		"volume.size":       _#PositiveGiBString
	}
}

// #Deployment derives one concrete, fail-closed Incus baseline and the
// controller fields that must agree with it from #Inputs. Security-sensitive
// values are exact constraints rather than defaults.
#Deployment: {
	// inputs supplies the complete operator-controlled deployment configuration.
	inputs!: #Inputs

	_runnerCPU:       inputs.runners.maximum * inputs.runners.cpu
	_runnerMemoryGiB: inputs.runners.maximum * inputs.runners.memoryGiB
	_runnerDiskGiB:   inputs.runners.maximum*inputs.runners.rootDiskGiB + inputs.runners.imageCacheGiB

	_cpuHeadroom:        inputs.host.cpu - _runnerCPU
	_cpuHeadroom:        >=inputs.host.reserve.cpu
	_memoryHeadroomGiB:  inputs.host.memoryGiB - _runnerMemoryGiB
	_memoryHeadroomGiB:  >=inputs.host.reserve.memoryGiB
	_storageHeadroomGiB: inputs.host.storageGiB - _runnerDiskGiB
	_storageHeadroomGiB: >=inputs.host.reserve.storageGiB

	// controller is a partial controller configuration. Merge these fields into
	// the deployment configuration so the controller cannot request more VMs or
	// select a different project/profile than the Incus baseline permits.
	controller: {
		// incus contains the controller fields coupled to the rendered baseline.
		incus: {
			// project selects the restricted project that owns runner VMs.
			project: inputs.names.project
			// profiles is the exact profile source pinned and materialized into runner VMs.
			profiles: [inputs.names.profile]
		}
		// capacity contains controller capacity coupled to Incus project limits.
		capacity: {
			// max_runners is the maximum concurrent runner count.
			max_runners: inputs.runners.maximum
		}
	}

	// output is the complete desired-state baseline consumed by the drift validator.
	output: _#Baseline & {
		names: {
			project:      inputs.names.project
			network:      inputs.names.network
			network_acl:  inputs.names.networkACL
			profile:      inputs.names.profile
			storage_pool: inputs.names.storagePool
		}
		project: config: {
			"limits.cpu":       "\(_runnerCPU)"
			"limits.disk":      "\(_runnerDiskGiB)GiB"
			"limits.instances": "\(inputs.runners.maximum)"
			"limits.memory":    "\(_runnerMemoryGiB)GiB"
		}
		network: config: {
			"dns.nameservers": inputs.network.dnsAddress
			"ipv4.address":    inputs.network.ipv4Address
		}
		network_acl: {
			egress: [
				{},
				{},
				{
					destination:      "\(inputs.network.proxy.address)/32"
					destination_port: "\(inputs.network.proxy.port)"
				},
				for endpoint in inputs.network.additionalEgress {
					description:      "Controlled additional egress: \(endpoint.name)"
					destination:      "\(endpoint.address)/32"
					protocol:         endpoint.protocol
					destination_port: "\(endpoint.port)"
				},
			]
		}
		profile: {
			config: {
				"limits.cpu":    "\(inputs.runners.cpu)"
				"limits.memory": "\(inputs.runners.memoryGiB)GiB"
			}
			devices: {
				eth0: {
					"limits.max":   "\(inputs.runners.networkMbit)Mbit"
					"ipv6.address": "none"
				}
				root: {
					size:         "\(inputs.runners.rootDiskGiB)GiB"
					"limits.max": "\(inputs.runners.diskIOMiB)MiB"
				}
			}
		}
		storage_pool: {
			driver: inputs.storage.driver
			config: source: inputs.storage.source
		}
	}

	if len(inputs.network.additionalEgress) == 0 {
		output: network_acl: description: "Default-deny runner egress through controlled DNS and HTTPS proxy"
	}

	if len(inputs.network.additionalEgress) > 0 {
		output: network_acl: description: "Default-deny runner egress through controlled endpoints"
	}

	if inputs.storage.driver == "lvm" {
		output: storage_pool: config: {
			"lvm.thinpool_name": inputs.storage.thinPoolName
			"volume.size":       "\(inputs.storage.volumeSizeGiB)GiB"
		}
	}
}
