package liveacceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ipv6ProbePort         = 18081
	curlCouldNotConnect   = 7
	curlOperationTimedOut = 28
)

// ipv6Observation records the self-assigned and link-local no-bypass probes.
type ipv6Observation struct {
	AddressA          string `json:"address_a"`
	AddressB          string `json:"address_b"`
	SpoofedSource     string `json:"spoofed_source"`
	LinkLocalB        string `json:"link_local_b"`
	SelfURLControl    bool   `json:"self_url_control"`
	SourceBindControl bool   `json:"source_bind_control"`
	ScopedURLControl  bool   `json:"scoped_url_control"`
	SelfAssignedBlock bool   `json:"self_assigned_blocked"`
	SpoofedBlock      bool   `json:"spoofed_source_blocked"`
	LinkLocalBlock    bool   `json:"link_local_blocked"`
	IPv4Recovered     bool   `json:"approved_ipv4_recovered"`
}

// guestRoute contains the interface selected by the guest's IPv4 default route.
type guestRoute struct {
	Device string `json:"dev"`
}

// guestAddressState contains the subset of `ip -json address` used by the IPv6 probe.
type guestAddressState struct {
	Addresses []struct {
		Local string `json:"local"`
		Scope string `json:"scope"`
	} `json:"addr_info"`
}

// proveIPv6NoBypass shows self-assigned, alternate-source, and link-local IPv6 cannot bypass the deployed profile.
func proveIPv6NoBypass(
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	options Options,
) (ipv6Observation, error) {
	interfaceA, interfaceErr := guestDefaultInterface(ctx, commands, options.Project, options.VMA)
	if interfaceErr != nil {
		return ipv6Observation{}, interfaceErr
	}
	interfaceB, interfaceErr := guestDefaultInterface(ctx, commands, options.Project, options.VMB)
	if interfaceErr != nil {
		return ipv6Observation{}, interfaceErr
	}
	initialA, hasGlobalA, stateErr := guestIPv6State(ctx, commands, options.Project, options.VMA, interfaceA)
	if stateErr != nil {
		return ipv6Observation{}, stateErr
	}
	initialB, hasGlobalB, stateErr := guestIPv6State(ctx, commands, options.Project, options.VMB, interfaceB)
	if stateErr != nil {
		return ipv6Observation{}, stateErr
	}
	if hasGlobalA || hasGlobalB {
		return ipv6Observation{}, errors.New("runner received a global IPv6 address before the hostile probe")
	}
	if writeErr := writer.write("ipv6-address-a-initial.json", initialA); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	if writeErr := writer.write("ipv6-address-b-initial.json", initialB); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	linkLocalA, addressErr := guestLinkLocalAddress(ctx, commands, options.Project, options.VMA, interfaceA)
	if addressErr != nil {
		return ipv6Observation{}, addressErr
	}
	linkLocalB, addressErr := guestLinkLocalAddress(ctx, commands, options.Project, options.VMB, interfaceB)
	if addressErr != nil {
		return ipv6Observation{}, addressErr
	}
	hextet := ipv6RunHextet(options.RunID)
	addressA := fmt.Sprintf("2001:db8:%s::a", hextet)
	addressB := fmt.Sprintf("2001:db8:%s::b", hextet)
	spoofedSource := fmt.Sprintf("2001:db8:%s::dead", hextet)

	observation, probeErr := executeIPv6NoBypass(
		ctx,
		commands,
		writer,
		options,
		interfaceA,
		interfaceB,
		addressA,
		addressB,
		spoofedSource,
		linkLocalA,
		linkLocalB,
	)
	cleanupErr := cleanupIPv6Probe(
		ctx,
		commands,
		options,
		interfaceA,
		interfaceB,
		addressA,
		addressB,
		spoofedSource,
	)
	return observation, errors.Join(probeErr, cleanupErr)
}

// executeIPv6NoBypass runs ordered positive controls and cross-runner negative probes.
func executeIPv6NoBypass( //nolint:funlen,gocognit // The retained assertions form one network experiment.
	ctx context.Context,
	commands commandRunner,
	writer *evidenceWriter,
	options Options,
	interfaceA string,
	interfaceB string,
	addressA string,
	addressB string,
	spoofedSource string,
	linkLocalA string,
	linkLocalB string,
) (ipv6Observation, error) {
	for _, address := range []struct {
		instance string
		device   string
		value    string
	}{
		{instance: options.VMA, device: interfaceA, value: addressA},
		{instance: options.VMA, device: interfaceA, value: spoofedSource},
		{instance: options.VMB, device: interfaceB, value: addressB},
	} {
		if _, addErr := guestOutput(
			ctx,
			commands,
			options.Project,
			address.instance,
			"ip",
			"-6",
			"address",
			"add",
			address.value+"/64",
			"dev",
			address.device,
			"nodad",
		); addErr != nil {
			return ipv6Observation{}, addErr
		}
	}

	for _, instance := range []string{options.VMA, options.VMB} {
		if listenerErr := startIPv6Listener(ctx, commands, options.Project, instance); listenerErr != nil {
			return ipv6Observation{}, listenerErr
		}
	}
	localURLA := fmt.Sprintf("http://[%s]:%d/", addressA, ipv6ProbePort)
	localURLB := fmt.Sprintf("http://[%s]:%d/", addressB, ipv6ProbePort)
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMA, localURLA, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("runner A IPv6 listener positive control: %w", waitErr)
	}
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMB, localURLB, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("runner B IPv6 listener positive control: %w", waitErr)
	}
	if waitErr := waitForGuestURL(
		ctx,
		commands,
		options.Project,
		options.VMA,
		localURLA,
		spoofedSource,
	); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("IPv6 source-binding positive control: %w", waitErr)
	}
	localLinkURLA := scopedIPv6URL(linkLocalA, interfaceA)
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMA, localLinkURLA, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("scoped link-local URL positive control: %w", waitErr)
	}
	localLinkURLB := scopedIPv6URL(linkLocalB, interfaceB)
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMB, localLinkURLB, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("runner B scoped link-local positive control: %w", waitErr)
	}

	selfAssigned, probeErr := blockedGuestURL(ctx, commands, options.Project, options.VMA, localURLB, "")
	if probeErr != nil {
		return ipv6Observation{}, probeErr
	}
	if !selfAssigned.blocked {
		return ipv6Observation{}, errors.New("protected runner reached a peer over self-assigned IPv6")
	}
	if writeErr := writer.write("ipv6-self-assigned-block.log", selfAssigned.output); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	spoofed, probeErr := blockedGuestURL(ctx, commands, options.Project, options.VMA, localURLB, spoofedSource)
	if probeErr != nil {
		return ipv6Observation{}, probeErr
	}
	if !spoofed.blocked {
		return ipv6Observation{}, errors.New("protected runner reached a peer with an alternate IPv6 source")
	}
	if writeErr := writer.write("ipv6-spoofed-source-block.log", spoofed.output); writeErr != nil {
		return ipv6Observation{}, writeErr
	}

	linkLocalURL := scopedIPv6URL(linkLocalB, interfaceA)
	linkLocal, probeErr := blockedGuestURL(ctx, commands, options.Project, options.VMA, linkLocalURL, "")
	if probeErr != nil {
		return ipv6Observation{}, probeErr
	}
	if !linkLocal.blocked {
		return ipv6Observation{}, errors.New("protected runner reached a peer over link-local IPv6")
	}
	if writeErr := writer.write("ipv6-link-local-block.log", linkLocal.output); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMB, localURLB, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("runner B listener post-denial positive control: %w", waitErr)
	}
	if waitErr := waitForGuestURL(ctx, commands, options.Project, options.VMB, localLinkURLB, ""); waitErr != nil {
		return ipv6Observation{}, fmt.Errorf("runner B scoped link-local post-denial control: %w", waitErr)
	}
	neighbor, _ := guestOutput(
		ctx,
		commands,
		options.Project,
		options.VMA,
		"ip",
		"-6",
		"neighbor",
		"show",
		"to",
		addressB,
	)
	if writeErr := writer.write("ipv6-neighbor-a-to-b.txt", []byte(neighbor)); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	stateA, _ := guestOutput(
		ctx,
		commands,
		options.Project,
		options.VMA,
		"ip",
		"-json",
		"-6",
		"address",
		"show",
		"dev",
		interfaceA,
	)
	stateB, _ := guestOutput(
		ctx,
		commands,
		options.Project,
		options.VMB,
		"ip",
		"-json",
		"-6",
		"address",
		"show",
		"dev",
		interfaceB,
	)
	if writeErr := writer.write("ipv6-address-a.json", []byte(stateA)); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	if writeErr := writer.write("ipv6-address-b.json", []byte(stateB)); writeErr != nil {
		return ipv6Observation{}, writeErr
	}
	if egressErr := guestEgress(
		ctx,
		commands,
		options.Project,
		options.VMB,
		options.AllowedURL,
		options.EgressProxy,
	); egressErr != nil {
		return ipv6Observation{}, fmt.Errorf(
			"approved IPv4 egress did not recover after IPv6 probes: %w",
			egressErr,
		)
	}
	return ipv6Observation{
		AddressA:          addressA,
		AddressB:          addressB,
		SpoofedSource:     spoofedSource,
		LinkLocalB:        linkLocalB,
		SelfURLControl:    true,
		SourceBindControl: true,
		ScopedURLControl:  true,
		SelfAssignedBlock: true,
		SpoofedBlock:      true,
		LinkLocalBlock:    true,
		IPv4Recovered:     true,
	}, nil
}

// startIPv6Listener starts the fixed synthetic HTTP listener inside one runner.
func startIPv6Listener(ctx context.Context, commands commandRunner, project string, instance string) error {
	_, err := guestOutput(ctx, commands, project, instance, "bash", "-c", ipv6ListenerScript())
	return err
}

// ipv6ListenerScript returns the synthetic listener lifecycle script.
func ipv6ListenerScript() string {
	return fmt.Sprintf(
		`set -Eeuo pipefail; printf '#!/bin/sh\nprintf "HTTP/1.1 200 OK\\r\\nConnection: close\\r\\nContent-Length: 21\\r\\n\\r\\nipv6-isolation-probe\\n"\n' >/run/incus-gh-runner-ipv6-response; chmod 0700 /run/incus-gh-runner-ipv6-response; nohup systemd-socket-activate --listen='[::]:%d' --accept --inetd -- /bin/sh /run/incus-gh-runner-ipv6-response >/run/incus-gh-runner-ipv6-listener.log 2>&1 </dev/null & printf '%%s\n' "$!" >/run/incus-gh-runner-ipv6-listener.pid`,
		ipv6ProbePort,
	)
}

// scopedIPv6URL formats one literal link-local URL with an escaped guest interface zone.
func scopedIPv6URL(address string, device string) string {
	return fmt.Sprintf("http://[%s%%25%s]:%d/", address, url.PathEscape(device), ipv6ProbePort)
}

// guestIPv6State returns one guest NIC's raw address state and whether it includes a global address.
func guestIPv6State(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
	device string,
) ([]byte, bool, error) {
	raw, err := guestOutput(ctx, commands, project, instance, "ip", "-json", "-6", "address", "show", "dev", device)
	if err != nil {
		return nil, false, err
	}
	var states []guestAddressState
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		return nil, false, fmt.Errorf("decode guest IPv6 address state: %w", err)
	}
	for _, state := range states {
		for _, address := range state.Addresses {
			if address.Scope == "global" {
				return []byte(raw), true, nil
			}
		}
	}
	return []byte(raw), false, nil
}

// cleanupIPv6Probe removes only the exact listener and addresses created by this probe.
func cleanupIPv6Probe(
	ctx context.Context,
	commands commandRunner,
	options Options,
	interfaceA string,
	interfaceB string,
	addressA string,
	addressB string,
	spoofedSource string,
) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), commandTimeout)
	defer cancel()
	listenerCleanup := `set -Eeuo pipefail
if [[ -f /run/incus-gh-runner-ipv6-listener.pid ]]; then
  pid="$(cat /run/incus-gh-runner-ipv6-listener.pid)"
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
fi
rm -f -- /run/incus-gh-runner-ipv6-listener.pid /run/incus-gh-runner-ipv6-response /run/incus-gh-runner-ipv6-listener.log
[[ ! -e /run/incus-gh-runner-ipv6-listener.pid && ! -e /run/incus-gh-runner-ipv6-response ]]`
	var cleanupErr error
	for _, instance := range []string{options.VMA, options.VMB} {
		_, err := guestOutput(cleanupContext, commands, options.Project, instance, "bash", "-c", listenerCleanup)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for _, address := range []struct {
		instance string
		device   string
		value    string
	}{
		{instance: options.VMA, device: interfaceA, value: spoofedSource},
		{instance: options.VMA, device: interfaceA, value: addressA},
		{instance: options.VMB, device: interfaceB, value: addressB},
	} {
		_, err := guestOutput(
			cleanupContext,
			commands,
			options.Project,
			address.instance,
			"bash",
			"-c",
			`ip -6 address delete "$1/64" dev "$2" 2>/dev/null || true; ! ip -6 address show dev "$2" | grep -Fq "$1/64"`,
			"bash",
			address.value,
			address.device,
		)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	return cleanupErr
}

// blockedURLResult records whether a guest HTTP request was denied and its bounded output.
type blockedURLResult struct {
	blocked  bool
	exitCode int
	output   []byte
}

// blockedGuestURL requires one bounded literal-IPv6 request to fail.
func blockedGuestURL(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
	target string,
	source string,
) (blockedURLResult, error) {
	requestContext, cancel := context.WithTimeout(ctx, shortCommandTimeout)
	defer cancel()
	curlArguments := []string{
		curlCommand, "--globoff", curlSilentArgument, curlShowErrorArgument,
		"--noproxy", "*", "--connect-timeout", "3", curlMaxTimeArgument, "5",
	}
	if source != "" {
		curlArguments = append(curlArguments, "--interface", source)
	}
	curlArguments = append(curlArguments, target)
	arguments := []string{
		incusExecArgument, instance, "--", "bash", "-c",
		`set +e; output="$("$@" 2>&1)"; status=$?; printf '%d\n%s' "$status" "$output"`,
		"bash",
	}
	arguments = append(arguments, curlArguments...)
	result, err := commands.incus(requestContext, project, arguments...)
	return classifyGuestCurlDenial(result, err)
}

// classifyGuestCurlDenial accepts only explicit guest curl network-denial exits from a successful Incus exec.
func classifyGuestCurlDenial(result commandResult, runErr error) (blockedURLResult, error) {
	if runErr != nil {
		return blockedURLResult{}, runErr
	}
	if checkErr := requireSuccess("execute negative IPv6 request", result); checkErr != nil {
		return blockedURLResult{}, checkErr
	}
	exitLine, detail, found := bytes.Cut(result.stdout, []byte("\n"))
	if !found {
		return blockedURLResult{}, errors.New("negative IPv6 request omitted the guest curl exit code")
	}
	exitCode, parseErr := strconv.Atoi(strings.TrimSpace(string(exitLine)))
	if parseErr != nil {
		return blockedURLResult{}, errors.New("negative IPv6 request returned an invalid guest curl exit code")
	}
	output := fmt.Appendf(nil, "curl_exit=%d\n%s", exitCode, detail)
	if exitCode == 0 {
		return blockedURLResult{exitCode: exitCode, output: output}, nil
	}
	if exitCode != curlCouldNotConnect && exitCode != curlOperationTimedOut {
		return blockedURLResult{}, fmt.Errorf(
			"negative IPv6 request failed with non-network curl exit code %d",
			exitCode,
		)
	}
	return blockedURLResult{blocked: true, exitCode: exitCode, output: output}, nil
}

// waitForGuestURL requires a guest-local HTTP listener to become reachable before negative probes.
func waitForGuestURL(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
	target string,
	source string,
) error {
	for range 20 {
		requestContext, cancel := context.WithTimeout(ctx, listenerCommandTimeout)
		arguments := []string{
			incusExecArgument,
			instance,
			"--",
			curlCommand,
			"--globoff",
			"--fail",
			curlSilentArgument,
			curlShowErrorArgument,
			"--noproxy",
			"*",
			curlMaxTimeArgument,
			"2",
		}
		if source != "" {
			arguments = append(arguments, "--interface", source)
		}
		arguments = append(arguments, target)
		result, err := commands.incus(requestContext, project, arguments...)
		cancel()
		if err == nil && result.succeeded() && strings.TrimSpace(string(result.stdout)) == "ipv6-isolation-probe" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ipv6ListenerPollInterval):
		}
	}
	return errors.New("guest-local IPv6 listener did not become ready")
}

// guestDefaultInterface returns the guest NIC carrying its IPv4 default route.
func guestDefaultInterface(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
) (string, error) {
	raw, err := guestOutput(ctx, commands, project, instance, "ip", "-json", "route", "show", "default")
	if err != nil {
		return "", err
	}
	var routes []guestRoute
	if err := json.Unmarshal(
		[]byte(raw),
		&routes,
	); err != nil || len(routes) != 1 ||
		!localNamePattern.MatchString(routes[0].Device) {
		return "", errors.New("guest returned an invalid default-route interface")
	}
	return routes[0].Device, nil
}

// guestLinkLocalAddress returns one link-local IPv6 address on the selected guest NIC.
func guestLinkLocalAddress(
	ctx context.Context,
	commands commandRunner,
	project string,
	instance string,
	device string,
) (string, error) {
	raw, err := guestOutput(ctx, commands, project, instance, "ip", "-json", "-6", "address", "show", "dev", device)
	if err != nil {
		return "", err
	}
	var states []guestAddressState
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		return "", fmt.Errorf("decode guest IPv6 address state: %w", err)
	}
	for _, state := range states {
		for _, address := range state.Addresses {
			if address.Scope == "link" && strings.HasPrefix(address.Local, "fe80:") {
				return address.Local, nil
			}
		}
	}
	return "", errors.New("guest has no link-local IPv6 address")
}

// ipv6RunHextet derives one deterministic documentation-prefix subnet from the harness run ID.
func ipv6RunHextet(runID string) string {
	digest := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(digest[:2])
}

// guestEgress requires the approved HTTP origin to remain reachable through the configured path.
func guestEgress(
	ctx context.Context,
	commands incusCommandRunner,
	project string,
	instance string,
	target string,
	proxy string,
) error {
	requestContext, cancel := context.WithTimeout(ctx, guestEgressTimeout)
	defer cancel()
	arguments := []string{
		incusExecArgument,
		instance,
		"--",
		curlCommand,
		"--fail",
		curlSilentArgument,
		curlShowErrorArgument,
		"--location",
		"--connect-timeout",
		"10",
		curlMaxTimeArgument,
		"30",
		"--output",
		"/dev/null",
	}
	if proxy == "" {
		arguments = append(arguments, "--noproxy", "*")
	} else {
		arguments = append(arguments, "--noproxy", "", "--proxy", proxy)
	}
	arguments = append(arguments, target)
	result, err := commands.incus(requestContext, project, arguments...)
	if err != nil {
		return err
	}
	return requireSuccess("probe approved guest egress", result)
}
