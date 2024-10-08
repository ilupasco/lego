package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/providers/dns/internal/errutils"
)

type Client struct {
	Token      string
	Email      string
	URL        *url.URL
	HTTPClient *http.Client
}

// NewClient Creates a new Client.
func NewClient(email, token, baseURL string) *Client {
	uri, _ := url.Parse(baseURL)
	return &Client{
		Email:      email,
		Token:      token,
		URL:        uri,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// AddRecord adds a DNS record.
func (c *Client) AddRecord(ctx context.Context, zone string, record Record) (*Response, error) {
	endpoint := c.URL.JoinPath("domain", dns01.UnFqdn(zone), "record")

	req, err := newJSONRequest(ctx, http.MethodPost, endpoint, record)
	if err != nil {
		return nil, err
	}
	respData := &Response{}
	err = c.do(req, respData)
	if err != nil {
		return nil, fmt.Errorf("add record: %w", err)
	}
	return respData, nil
}

// RemoveRecord removes a DNS record.
func (c *Client) RemoveRecord(ctx context.Context, zone string, id int) error {
	endpoint := c.URL.JoinPath("domain", dns01.UnFqdn(zone), "record", strconv.Itoa(id))
	req, err := newJSONRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	err = c.do(req, nil)
	if err != nil {
		return fmt.Errorf("remove record: %w", err)
	}
	return nil
}

func (c *Client) do(req *http.Request, result any) error {
	req.Header.Set(HeaderAuthEmail, c.Email)
	req.Header.Set(HeaderAuthToken, c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return errutils.NewHTTPDoError(req, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		return parseError(req, resp)
	}

	if result == nil {
		return nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return errutils.NewReadResponseError(req, resp.StatusCode, err)
	}

	err = json.Unmarshal(raw, result)
	if err != nil {
		return errutils.NewUnmarshalError(req, resp.StatusCode, raw, err)
	}

	return nil
}

func newJSONRequest(ctx context.Context, method string, endpoint *url.URL, payload any) (*http.Request, error) {
	buf := new(bytes.Buffer)

	if payload != nil {
		err := json.NewEncoder(buf).Encode(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to create request JSON body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), buf)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func parseError(req *http.Request, resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)

	var errAPI APIError
	err := json.Unmarshal(raw, &errAPI)
	if err != nil {
		return errutils.NewUnexpectedStatusCodeError(req, resp.StatusCode, raw)
	}

	return fmt.Errorf("[status code: %d] %w", resp.StatusCode, errAPI)
}
