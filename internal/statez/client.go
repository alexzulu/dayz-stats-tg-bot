package statez

import "gh.tarampamp.am/dayz-statez/server/pkg/api"

// NewClient creates a new [api.Client] with the given base URL and auth token, and applies the provided options.
//
// NOTE: It also adds [api.WithInsecureTLS] to the options, which disables TLS certificate verification on the
// underlying HTTP client.
func NewClient(baseUrl, authToken string, opts ...api.ClientOption) *api.Client {
	return api.NewClient(baseUrl, authToken, append(opts, api.WithInsecureTLS())...)
}
