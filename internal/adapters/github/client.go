// Package github adapts the GitHub Actions runner scale-set client.
package github

import "github.com/actions/scaleset"

// NewClientWithGitHubApp constructs a scale-set client using GitHub App credentials.
func NewClientWithGitHubApp(
	config scaleset.ClientWithGitHubAppConfig,
	options ...scaleset.HTTPOption,
) (*scaleset.Client, error) {
	return scaleset.NewClientWithGitHubApp(config, options...)
}

// NewClientWithPersonalAccessToken constructs a scale-set client using a personal access token.
func NewClientWithPersonalAccessToken(
	config scaleset.NewClientWithPersonalAccessTokenConfig,
	options ...scaleset.HTTPOption,
) (*scaleset.Client, error) {
	return scaleset.NewClientWithPersonalAccessToken(config, options...)
}
