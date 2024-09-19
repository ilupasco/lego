// Package mdns implements a DNS provider for solving the DNS-01 challenge using UKFast SafeDNS.
package mdns

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/platform/config/env"
	"github.com/go-acme/lego/v4/providers/dns/mdns/internal"
)

// Environment variables.
const (
	envNamespace = "MDNS_"
	EnvAuthEmail = envNamespace + "AUTH_EMAIL"
	EnvAuthKey   = envNamespace + "AUTH_KEY"
	EnvBaseURL   = envNamespace + "BASE_URL"

	EnvTTL                = envNamespace + "TTL"
	EnvPropagationTimeout = envNamespace + "PROPAGATION_TIMEOUT"
	EnvPollingInterval    = envNamespace + "POLLING_INTERVAL"
	EnvHTTPTimeout        = envNamespace + "HTTP_TIMEOUT"
)

// Config is used to configure the creation of the DNSProvider.
type Config struct {
	AuthEmail string
	AuthKey   string
	BaseURL   string

	TTL                int
	PropagationTimeout time.Duration
	PollingInterval    time.Duration
	HTTPClient         *http.Client
}

// NewDefaultConfig returns a default configuration for the DNSProvider.
func NewDefaultConfig() *Config {
	return &Config{
		TTL:                env.GetOrDefaultInt(EnvTTL, dns01.DefaultTTL),
		PropagationTimeout: env.GetOrDefaultSecond(EnvPropagationTimeout, dns01.DefaultPropagationTimeout),
		PollingInterval:    env.GetOrDefaultSecond(EnvPollingInterval, dns01.DefaultPollingInterval),
		HTTPClient: &http.Client{
			Timeout: env.GetOrDefaultSecond(EnvHTTPTimeout, 30*time.Second),
		},
	}
}

// DNSProvider implements the challenge.Provider interface.
type DNSProvider struct {
	config    *Config
	client    *internal.Client
	recordsID sync.Map
}

// NewDNSProvider returns a DNSProvider instance.
func NewDNSProvider() (*DNSProvider, error) {
	values, err := env.Get(EnvAuthEmail, EnvAuthKey, EnvBaseURL)
	if err != nil {
		return nil, fmt.Errorf("mdns: %w", err)
	}
	config := NewDefaultConfig()

	config.AuthEmail = values[EnvAuthEmail]
	config.AuthKey = values[EnvAuthKey]
	config.BaseURL = values[EnvBaseURL]

	return NewDNSProviderConfig(config)
}

// NewDNSProviderConfig return a DNSProvider instance configured for UKFast SafeDNS.
func NewDNSProviderConfig(config *Config) (*DNSProvider, error) {
	if config == nil {
		return nil, errors.New("mdns: supplied configuration was nil")
	}
	if config.AuthKey == "" || config.AuthEmail == "" {
		return nil, errors.New("mdns: credentials missing")
	}
	if config.BaseURL == "" {
		config.BaseURL = internal.DefaultBaseURL
	}

	client := internal.NewClient(config.AuthEmail, config.AuthKey, config.BaseURL)

	if config.HTTPClient != nil {
		client.HTTPClient = config.HTTPClient
	}

	return &DNSProvider{
		config: config,
		client: client,
	}, nil
}

// Timeout returns the timeout and interval to use when checking for DNS propagation.
// Adjusting here to cope with spikes in propagation times.
func (d *DNSProvider) Timeout() (timeout, interval time.Duration) {
	return d.config.PropagationTimeout, d.config.PollingInterval
}

// Present creates a TXT record to fulfill the dns-01 challenge.
func (d *DNSProvider) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)

	zone, err := dns01.FindZoneByFqdn(dns01.ToFqdn(info.EffectiveFQDN))
	if err != nil {
		return fmt.Errorf("mdns: could not find zone for domain %q: %w", domain, err)
	}

	record := internal.Record{
		Name:    dns01.UnFqdn(info.EffectiveFQDN),
		Type:    "TXT",
		Content: fmt.Sprintf("%q", info.Value),
	}

	res, err := d.client.AddRecord(context.Background(), zone, record)
	if err != nil {
		return fmt.Errorf("mdns: %w", err)
	}
	id := res.Results[0].ID
	d.recordsID.Store(token, id)
	return nil
}

// CleanUp removes the TXT record previously created.
func (d *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)

	zone, err := dns01.FindZoneByFqdn(info.EffectiveFQDN)
	if err != nil {
		return fmt.Errorf("mdns: could not find zone for domain %q: %w", domain, err)
	}
	id, ok := d.recordsID.Load(token)
	if !ok {
		return fmt.Errorf("mdns: unknown ref for %s", info.EffectiveFQDN)
	}

	err = d.client.RemoveRecord(context.Background(), zone, id.(int))
	if err != nil {
		return fmt.Errorf("mdns: %w", err)
	}
	d.recordsID.Delete(token)
	return nil
}
