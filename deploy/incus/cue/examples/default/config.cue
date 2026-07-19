package defaultconfig

import runner "github.com/meigma/incus-gh-runner/config@v0:incusrunner"

_deployment: runner.#Deployment & {
	inputs: {
		host: {
			cpu:        24
			memoryGiB:  48
			storageGiB: 260
		}
		network: {
			ipv4Address: "10.248.0.1/24"
			dnsAddress:  "192.0.2.53"
			proxy: address: "192.0.2.10"
		}
		storage: source: "incus-gh-runners"
	}
}

// baseline is the rendered default Incus desired-state manifest.
baseline: _deployment.output
// controller is the rendered controller configuration fragment aligned with baseline.
controller: _deployment.controller
