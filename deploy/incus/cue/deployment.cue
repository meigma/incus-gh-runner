package incusrunner

import "net"

#Name: (string & =~"^[a-z][a-z0-9-]{0,62}$") |
		error("name must be a lowercase Incus identifier of at most 63 characters")
#DedicatedName: (#Name & !="default") |
		error("dedicated Incus resource name must not be default")
#PositiveInt: (int & >=1) | error("value must be a positive integer")
#ProxyPort:   (int & >=1 & <=65535 & !=53) |
		error("proxy port must be between 1 and 65535 and must not be the DNS port")
#IPv4:          (net.IP & !~":") | error("value must be an IPv4 address")
#IPv4CIDR:      (net.IPCIDR & !~":") | error("value must be an IPv4 CIDR")
#StorageSource: (string & =~"^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,254}$") |
	error("storage source contains unsupported characters")

// #Inputs is the complete operator-controlled configuration surface. Fields
// absent from this definition are deliberately not configurable.
#Inputs: {
	names: {
		project:     #DedicatedName & (*"github-runners" | string)
		network:     #DedicatedName & (*"runner-network" | string)
		networkACL:  #DedicatedName & (*"runner-egress" | string)
		profile:     #DedicatedName & (*"github-runner" | string)
		storagePool: #DedicatedName & (*"runner-storage" | string)
	}

	host: {
		cpu!:        #PositiveInt
		memoryGiB!:  #PositiveInt
		storageGiB!: #PositiveInt
		reserve: {
			cpu:        #PositiveInt & (*4 | int)
			memoryGiB:  #PositiveInt & (*8 | int)
			storageGiB: #PositiveInt & (*40 | int)
		}
	}

	runners: {
		maximum:       (int & >=1 & <=100) & (*10 | int)
		cpu:           (int & >=1 & <=64) & (*2 | int)
		memoryGiB:     (int & >=1 & <=1024) & (*4 | int)
		rootDiskGiB:   (int & >=1 & <=16384) & (*20 | int)
		networkMbit:   (int & >=1 & <=100000) & (*100 | int)
		diskIOMiB:     (int & >=1 & <=100000) & (*100 | int)
		imageCacheGiB: (int & >=1 & <=16384) & (*20 | int)
	}

	network: {
		ipv4Address!: #IPv4CIDR
		dnsAddress!:  #IPv4
		proxy: {
			address!: #IPv4
			port:     #ProxyPort & (*3128 | int)
		}
	}

	storage: source!: #StorageSource
}

// #Deployment derives one concrete, fail-closed Incus baseline and the
// controller fields that must agree with it from #Inputs. Security-sensitive
// values are exact constraints rather than defaults.
#Deployment: {
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
		incus: {
			project: inputs.names.project
			profiles: [inputs.names.profile]
		}
		capacity: max_runners: inputs.runners.maximum
	}

	output: {
		schema_version: 1
		authority: {
			mode:                                   "dedicated-host-unix-socket"
			dedicated_single_purpose_host_required: true
			unix_socket_is_root_equivalent:         true
		}
		names: {
			project:      inputs.names.project
			network:      inputs.names.network
			network_acl:  inputs.names.networkACL
			profile:      inputs.names.profile
			storage_pool: inputs.names.storagePool
		}
		server: {
			minimum_version: "7.0"
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
			firewall_driver:       "nftables"
			standalone:            true
			core_https_address:    ""
			cluster_https_address: ""
		}
		residual_controls: project_vm_nesting_restriction: {
			status:                     "unsupported-by-incus-7.0-through-7.2"
			future_api_extension:       "projects_restricted_virtual_machines_nesting"
			compensating_profile_key:   "security.nesting"
			compensating_profile_value: "false"
		}
		project: {
			description: "Restricted project for ephemeral GitHub runner VMs"
			config: {
				"features.images":                                "true"
				"features.networks":                              "false"
				"features.profiles":                              "true"
				"features.storage.buckets":                       "true"
				"features.storage.volumes":                       "true"
				"images.auto_update_cached":                      "false"
				"images.auto_update_interval":                    "0"
				"limits.containers":                              "0"
				"limits.cpu":                                     "\(_runnerCPU)"
				"limits.disk":                                    "\(_runnerDiskGiB)GiB"
				("limits.disk.pool." + inputs.names.storagePool): "\(_runnerDiskGiB)GiB"
				"limits.instances":                               "\(inputs.runners.maximum)"
				"limits.memory":                                  "\(_runnerMemoryGiB)GiB"
				"limits.networks":                                "0"
				"limits.virtual-machines":                        "\(inputs.runners.maximum)"
				"restricted":                                     "true"
				"restricted.backups":                             "block"
				"restricted.cluster.target":                      "block"
				"restricted.containers.interception":             "block"
				"restricted.containers.lowlevel":                 "block"
				"restricted.containers.nesting":                  "block"
				"restricted.containers.privilege":                "unprivileged"
				"restricted.devices.disk":                        "block"
				"restricted.devices.gpu":                         "block"
				"restricted.devices.infiniband":                  "block"
				"restricted.devices.nic":                         "managed"
				"restricted.devices.pci":                         "block"
				"restricted.devices.proxy":                       "block"
				"restricted.devices.unix-block":                  "block"
				"restricted.devices.unix-char":                   "block"
				"restricted.devices.unix-hotplug":                "block"
				"restricted.devices.usb":                         "block"
				"restricted.networks.access":                     inputs.names.network
				"restricted.snapshots":                           "block"
				"restricted.storage-pools.access":                inputs.names.storagePool
				"restricted.virtual-machines.lowlevel":           "block"
			}
		}
		network: {
			description: "Managed bridge for isolated ephemeral runner VMs"
			type:        "bridge"
			managed:     true
			config: {
				"bridge.driver":                        "native"
				"dns.mode":                             "none"
				"dns.nameservers":                      inputs.network.dnsAddress
				"ipv4.address":                         inputs.network.ipv4Address
				"ipv4.dhcp":                            "true"
				"ipv4.firewall":                        "true"
				"ipv4.nat":                             "true"
				"ipv4.routing":                         "true"
				"ipv6.address":                         "none"
				"raw.dnsmasq":                          "port=0"
				"security.acls":                        inputs.names.networkACL
				"security.acls.default.egress.action":  "reject"
				"security.acls.default.egress.logged":  "true"
				"security.acls.default.ingress.action": "reject"
				"security.acls.default.ingress.logged": "true"
			}
		}
		network_acl: {
			description: "Default-deny runner egress through controlled DNS and HTTPS proxy"
			config: {}
			ingress: []
			egress: [
				{
					action:           "allow"
					state:            "enabled"
					description:      "Controlled DNS over UDP"
					destination:      "\(inputs.network.dnsAddress)/32"
					protocol:         "udp"
					destination_port: "53"
				},
				{
					action:           "allow"
					state:            "enabled"
					description:      "Controlled DNS over TCP"
					destination:      "\(inputs.network.dnsAddress)/32"
					protocol:         "tcp"
					destination_port: "53"
				},
				{
					action:           "allow"
					state:            "enabled"
					description:      "Controlled HTTP CONNECT proxy"
					destination:      "\(inputs.network.proxy.address)/32"
					protocol:         "tcp"
					destination_port: "\(inputs.network.proxy.port)"
				},
			]
		}
		profile: {
			description: "Only profile applied to ephemeral GitHub runner VMs"
			config: {
				"boot.autostart":      "false"
				"limits.cpu":          "\(inputs.runners.cpu)"
				"limits.memory":       "\(inputs.runners.memoryGiB)GiB"
				"security.guestapi":   "false"
				"security.nesting":    "false"
				"security.secureboot": "true"
			}
			devices: {
				eth0: {
					type:                                   "nic"
					network:                                inputs.names.network
					"limits.max":                           "\(inputs.runners.networkMbit)Mbit"
					"security.acls":                        inputs.names.networkACL
					"security.acls.default.egress.action":  "reject"
					"security.acls.default.egress.logged":  "true"
					"security.acls.default.ingress.action": "reject"
					"security.acls.default.ingress.logged": "true"
					"security.ipv4_filtering":              "true"
					"security.ipv6_filtering":              "true"
					"security.mac_filtering":               "true"
					"security.port_isolation":              "true"
				}
				root: {
					type:         "disk"
					path:         "/"
					pool:         inputs.names.storagePool
					size:         "\(inputs.runners.rootDiskGiB)GiB"
					"limits.max": "\(inputs.runners.diskIOMiB)MiB"
				}
			}
		}
		storage_pool: {
			description: "Dedicated ZFS pool for ephemeral runner images and VM roots"
			driver:      "zfs"
			config: {
				source:          inputs.storage.source
				"zfs.pool_name": inputs.storage.source
			}
		}
	}
}
