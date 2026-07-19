package tests

import runner "github.com/meigma/incus-gh-runner/config@v0:incusrunner"

#FixtureInputs: {
	host: {
		cpu:        *24 | int
		memoryGiB:  *48 | int
		storageGiB: *260 | int
		reserve: {
			cpu:        *4 | int
			memoryGiB:  *8 | int
			storageGiB: *40 | int
		}
	}
	runners: {
		maximum:       *10 | int
		cpu:           *2 | int
		memoryGiB:     *4 | int
		rootDiskGiB:   *20 | int
		networkMbit:   *100 | int
		diskIOMiB:     *100 | int
		imageCacheGiB: *20 | int
	}
	network: {
		ipv4Address: *"10.248.0.1/24" | string
		dnsAddress:  *"192.0.2.53" | string
		proxy: {
			address: *"192.0.2.10" | string
			port:    *3128 | int
		}
		...
	}
	storage: source: *"incus-gh-runners" | string
	...
}

cases: {
	valid: inputs: #FixtureInputs
	customSizing: inputs: #FixtureInputs & {
		host: {
			cpu:        16
			memoryGiB:  32
			storageGiB: 160
		}
		runners: {
			maximum:       3
			cpu:           4
			memoryGiB:     8
			rootDiskGiB:   30
			networkMbit:   250
			diskIOMiB:     150
			imageCacheGiB: 30
		}
		network: proxy: port: 8080
	}
	unknownInput: inputs: #FixtureInputs & {
		unsafeRawIncusConfig: true
	}
	unknownNetworkInput: inputs: #FixtureInputs & {
		network: rawIncusConfig: "ipv4.nat=false"
	}
	defaultProject: inputs: #FixtureInputs & {
		names: project: "default"
	}
	invalidDNS: inputs: #FixtureInputs & {
		network: dnsAddress: "999.0.2.53"
	}
	proxyOnDNSPort: inputs: #FixtureInputs & {
		network: proxy: port: 53
	}
	insufficientCPUHeadroom: inputs: #FixtureInputs & {
		host: cpu: 23
	}
	insufficientMemoryHeadroom: inputs: #FixtureInputs & {
		host: memoryGiB: 47
	}
	insufficientStorageHeadroom: inputs: #FixtureInputs & {
		host: storageGiB: 259
	}
	weakenSecureBoot: {
		inputs: #FixtureInputs
		output: profile: config: "security.secureboot": "false"
	}
	weakenDefaultEgress: {
		inputs: #FixtureInputs
		output: network: config: "security.acls.default.egress.action": "allow"
	}
}

_case:   string @tag(case)
_result: runner.#Deployment & cases[_case]
