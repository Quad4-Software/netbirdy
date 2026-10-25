package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/netbirdio/netbird/shared/management/http/util"
)

// APIError represents an error response from the management API.
type APIError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.Message
}

// IsNotFound returns true if the error represents a 404 Not Found response.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if ok := errors.As(err, &apiErr); ok {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

// Client Management service HTTP REST API Client
type Client struct {
	managementURL string
	authHeader    string
	httpClient    HttpClient
	userAgent     string

	// Accounts NetBirdy account APIs
	// see more: https://docs.netbird.io/api/resources/accounts
	Accounts *AccountsAPI

	// Users NetBirdy users APIs
	// see more: https://docs.netbird.io/api/resources/users
	Users *UsersAPI

	// Tokens NetBirdy tokens APIs
	// see more: https://docs.netbird.io/api/resources/tokens
	Tokens *TokensAPI

	// Peers NetBirdy peers APIs
	// see more: https://docs.netbird.io/api/resources/peers
	Peers *PeersAPI

	// SetupKeys NetBirdy setup keys APIs
	// see more: https://docs.netbird.io/api/resources/setup-keys
	SetupKeys *SetupKeysAPI

	// Groups NetBirdy groups APIs
	// see more: https://docs.netbird.io/api/resources/groups
	Groups *GroupsAPI

	// Policies NetBirdy policies APIs
	// see more: https://docs.netbird.io/api/resources/policies
	Policies *PoliciesAPI

	// PostureChecks NetBirdy posture checks APIs
	// see more: https://docs.netbird.io/api/resources/posture-checks
	PostureChecks *PostureChecksAPI

	// Networks NetBirdy networks APIs
	// see more: https://docs.netbird.io/api/resources/networks
	Networks *NetworksAPI

	// Routes NetBirdy routes APIs
	// see more: https://docs.netbird.io/api/resources/routes
	Routes *RoutesAPI

	// DNS NetBirdy DNS APIs
	// see more: https://docs.netbird.io/api/resources/dns
	DNS *DNSAPI

	// DNSZones NetBirdy DNS Zones APIs
	// see more: https://docs.netbird.io/api/resources/dns-zones
	DNSZones *DNSZonesAPI

	// GeoLocation NetBirdy Geo Location APIs
	// see more: https://docs.netbird.io/api/resources/geo-locations
	GeoLocation *GeoLocationAPI

	// Events NetBirdy Events APIs
	// see more: https://docs.netbird.io/api/resources/events
	Events *EventsAPI

	// Billing NetBirdy Billing APIs for subscriptions, plans, and invoices
	// see more: https://docs.netbird.io/api/resources/billing
	Billing *BillingAPI

	// MSP NetBirdy MSP tenant management APIs
	// see more: https://docs.netbird.io/api/resources/msp
	MSP *MSPAPI

	// EDR NetBirdy EDR integration APIs (Intune, SentinelOne, Falcon, Huntress)
	// see more: https://docs.netbird.io/api/resources/edr
	EDR *EDRAPI

	// SCIM NetBirdy SCIM IDP integration APIs
	// see more: https://docs.netbird.io/api/resources/scim
	SCIM *SCIMAPI

	// GoogleIDP NetBirdy Google Workspace IDP integration APIs
	GoogleIDP *GoogleIDPAPI

	// AzureIDP NetBirdy Azure AD IDP integration APIs
	AzureIDP *AzureIDPAPI

	// OktaScimIDP NetBirdy Okta SCIM IDP integration APIs
	OktaScimIDP *OktaScimIDPAPI

	// EventStreaming NetBirdy Event Streaming integration APIs
	// see more: https://docs.netbird.io/api/resources/event-streaming
	EventStreaming *EventStreamingAPI

	// IdentityProviders NetBirdy Identity Providers APIs
	// see more: https://docs.netbird.io/api/resources/identity-providers
	IdentityProviders *IdentityProvidersAPI

	// Instance NetBirdy Instance API
	// see more: https://docs.netbird.io/api/resources/instance
	Instance *InstanceAPI

	// ReverseProxyServices NetBirdy reverse proxy services APIs
	ReverseProxyServices *ReverseProxyServicesAPI

	// ReverseProxyClusters NetBirdy reverse proxy clusters APIs
	ReverseProxyClusters *ReverseProxyClustersAPI

	// ReverseProxyDomains NetBirdy reverse proxy domains APIs
	ReverseProxyDomains *ReverseProxyDomainsAPI

	// ReverseProxyTokens account-scoped proxy access tokens used to register
	// self-hosted (bring-your-own-proxy) `netbirdy proxy` instances.
	ReverseProxyTokens *ReverseProxyTokensAPI

	// AgentNetwork NetBirdy Agent Network (AI/LLM gateway) APIs: catalog,
	// providers, policies, guardrails, budget rules and account settings.
	AgentNetwork *AgentNetworkAPI
}

// New initialize new Client instance using PAT token
func New(managementURL, token string) *Client {
	return NewWithOptions(
		WithManagementURL(managementURL),
		WithPAT(token),
	)
}

// NewWithBearerToken initialize new Client instance using Bearer token type
func NewWithBearerToken(managementURL, token string) *Client {
	return NewWithOptions(
		WithManagementURL(managementURL),
		WithBearerToken(token),
	)
}

// NewWithOptions initialize new Client instance with options
func NewWithOptions(opts ...option) *Client {
	client := &Client{
		httpClient: http.DefaultClient,
	}

	for _, option := range opts {
		option(client)
	}

	client.initialize()
	return client
}

func (c *Client) initialize() {
	c.Accounts = &AccountsAPI{c}
	c.Users = &UsersAPI{c}
	c.Tokens = &TokensAPI{c}
	c.Peers = &PeersAPI{c}
	c.SetupKeys = &SetupKeysAPI{c}
	c.Groups = &GroupsAPI{c}
	c.Policies = &PoliciesAPI{c}
	c.PostureChecks = &PostureChecksAPI{c}
	c.Networks = &NetworksAPI{c}
	c.Routes = &RoutesAPI{c}
	c.DNS = &DNSAPI{c}
	c.DNSZones = &DNSZonesAPI{c}
	c.GeoLocation = &GeoLocationAPI{c}
	c.Events = &EventsAPI{c}
	c.Billing = &BillingAPI{c}
	c.MSP = &MSPAPI{c}
	c.EDR = &EDRAPI{c}
	c.SCIM = &SCIMAPI{c}
	c.GoogleIDP = &GoogleIDPAPI{c}
	c.AzureIDP = &AzureIDPAPI{c}
	c.OktaScimIDP = &OktaScimIDPAPI{c}
	c.EventStreaming = &EventStreamingAPI{c}
	c.IdentityProviders = &IdentityProvidersAPI{c}
	c.Instance = &InstanceAPI{c}
	c.ReverseProxyServices = &ReverseProxyServicesAPI{c}
	c.ReverseProxyClusters = &ReverseProxyClustersAPI{c}
	c.ReverseProxyDomains = &ReverseProxyDomainsAPI{c}
	c.ReverseProxyTokens = &ReverseProxyTokensAPI{c}
	c.AgentNetwork = &AgentNetworkAPI{c}
}

// NewRequest creates and executes new management API request
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader, query map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.managementURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", c.authHeader)
	req.Header.Add("Accept", "application/json")
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	if len(query) != 0 {
		q := req.URL.Query()
		for k, v := range query {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 {
		parsedErr, pErr := parseResponse[util.ErrorResponse](resp)
		if pErr != nil {
			return nil, pErr
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parsedErr.Message,
		}
	}

	return resp, nil
}

func parseResponse[T any](resp *http.Response) (T, error) {
	var ret T
	if resp.Body == nil {
		return ret, fmt.Errorf("body missing, HTTP Error code %d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return ret, err
	}
	err = json.Unmarshal(bs, &ret)
	if err != nil {
		return ret, fmt.Errorf("error code %d, error unmarshalling body: %w", resp.StatusCode, err)
	}

	return ret, nil
}
