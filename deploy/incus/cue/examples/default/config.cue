package defaultconfig

import runner "github.com/meigma/incus-gh-runner/config@v0:incusrunner"

deployment: runner.#Deployment & {
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

baseline:   deployment.output
controller: deployment.controller
