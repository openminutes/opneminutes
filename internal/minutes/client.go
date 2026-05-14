package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRegion       = "feishu"
	defaultBaseURL      = "https://meetings.feishu.cn"
	defaultSpaceBaseURL = "https://internal-api-space.feishu.cn"
	defaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"
	defaultLanguage     = "zh_cn"
)

// NewClient creates a Feishu Minutes client.
func NewClient(config Config) (*Client, error) {
	region := strings.TrimSpace(config.Region)
	if region == "" {
		region = defaultRegion
	}
	if region != "feishu" && region != "larksuite" {
		return nil, fmt.Errorf("invalid region %q: must be one of feishu, larksuite", region)
	}

	cookie := strings.TrimSpace(config.Cookie)
	if cookie == "" {
		return nil, errors.New("cookie is required")
	}

	csrfToken, err := csrfTokenFromCookie(cookie)
	if err != nil {
		return nil, err
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		var ok bool
		baseURL, ok = defaultBaseURLForRegion(region)
		if !ok {
			return nil, fmt.Errorf("base URL is required for region %q", region)
		}
	}

	spaceBaseURL := strings.TrimRight(strings.TrimSpace(config.SpaceBaseURL), "/")
	if spaceBaseURL == "" {
		spaceBaseURL = defaultSpaceBaseURL
	}

	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	return &Client{
		httpClient:   httpClient,
		baseURL:      baseURL,
		spaceBaseURL: spaceBaseURL,
		cookie:       cookie,
		csrfToken:    csrfToken,
		userAgent:    userAgent,
		referer:      baseURL + "/minutes/home",
	}, nil
}

func defaultBaseURLForRegion(region string) (string, bool) {
	switch region {
	case "feishu":
		return defaultBaseURL, true
	default:
		return "", false
	}
}

func csrfTokenFromCookie(cookie string) (string, error) {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if name == "bv_csrf_token" && value != "" {
			return value, nil
		}
	}

	return "", errors.New("cookie does not contain bv_csrf_token")
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("cookie", c.cookie)
	req.Header.Set("bv-csrf-token", c.csrfToken)
	req.Header.Set("referer", c.referer)
	req.Header.Set("user-agent", c.userAgent)

	return req, nil
}

func (c *Client) newAPIRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	return c.newRequest(ctx, method, c.apiURL(path, query), body)
}

func (c *Client) newSpaceRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	return c.newRequest(ctx, method, c.spaceURL(path, query), body)
}

func (c *Client) apiURL(path string, query url.Values) string {
	return buildURL(c.baseURL, path, query)
}

func (c *Client) spaceURL(path string, query url.Values) string {
	return buildURL(c.spaceBaseURL, path, query)
}

func buildURL(baseURL, path string, query url.Values) string {
	rawURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	if len(query) == 0 {
		return rawURL
	}

	return rawURL + "?" + query.Encode()
}

func (c *Client) doJSON(req *http.Request, result any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return httpStatusError(req, resp)
	}

	var envelope responseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		if errors.Is(err, io.EOF) && result == nil {
			return nil
		}
		return err
	}

	if envelope.Code != 0 {
		message := envelope.Msg
		if message == "" {
			message = envelope.Message
		}
		if message == "" {
			message = "request failed"
		}
		return fmt.Errorf("%s %s: server code %d: %s", req.Method, req.URL.RequestURI(), envelope.Code, message)
	}

	if result == nil {
		return nil
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return fmt.Errorf("%s %s: response missing data", req.Method, req.URL.RequestURI())
	}

	return json.Unmarshal(envelope.Data, result)
}

func (c *Client) doRaw(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, httpStatusError(req, resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if strings.Contains(resp.Header.Get("content-type"), "application/json") {
		var envelope responseEnvelope
		if err := json.Unmarshal(data, &envelope); err == nil && envelope.Code != 0 {
			message := envelope.Msg
			if message == "" {
				message = envelope.Message
			}
			if message == "" {
				message = "request failed"
			}
			return nil, fmt.Errorf("%s %s: server code %d: %s", req.Method, req.URL.RequestURI(), envelope.Code, message)
		}
	}

	return data, nil
}

func (c *Client) doStream(req *http.Request, dst io.Writer) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return httpStatusError(req, resp)
	}

	_, err = io.Copy(dst, resp.Body)
	return err
}

func httpStatusError(req *http.Request, resp *http.Response) error {
	return fmt.Errorf("%s %s: unexpected HTTP status %s", req.Method, req.URL.RequestURI(), resp.Status)
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
