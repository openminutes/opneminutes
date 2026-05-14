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
	"sort"
	"strings"
	"time"

	apperrors "openminutes/internal/errors"

	"go.uber.org/zap"
)

const (
	DefaultBaseURL       = "https://meetings.feishu.cn"
	DefaultSpaceBaseURL  = "https://internal-api-space.feishu.cn"
	defaultBaseURL       = DefaultBaseURL
	defaultSpaceBaseURL  = DefaultSpaceBaseURL
	defaultUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"
	defaultLanguage      = "zh_cn"
	defaultClientTimeout = 30 * time.Second
)

// NewClient creates a Feishu Minutes client.
func NewClient(config Config) (*Client, error) {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	cookie := strings.TrimSpace(config.Cookie)
	if cookie == "" {
		err := apperrors.New(apperrors.KindAuth, "cookie is required")
		logger.Debug("client initialization failed",
			zap.Bool("cookie_present", false),
			zap.Error(err),
		)
		return nil, err
	}

	csrfToken, err := csrfTokenFromCookie(cookie)
	if err != nil {
		logger.Debug("client initialization failed",
			zap.Bool("cookie_present", true),
			zap.Bool("csrf_token_present", false),
			zap.Error(err),
		)
		return nil, err
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}

	baseURL, baseURLDefaulted, err := NormalizeBaseURLOrDefault("base_url", config.BaseURL, defaultBaseURL)
	if err != nil {
		logger.Debug("client initialization failed",
			zap.String("base_url", baseURL),
			zap.Bool("base_url_defaulted", baseURLDefaulted),
			zap.Error(err),
		)
		return nil, apperrors.Wrap(apperrors.KindConfig, err)
	}

	spaceBaseURL, spaceBaseURLDefaulted, err := NormalizeBaseURLOrDefault("space_base_url", config.SpaceBaseURL, defaultSpaceBaseURL)
	if err != nil {
		logger.Debug("client initialization failed",
			zap.String("space_base_url", spaceBaseURL),
			zap.Bool("space_base_url_defaulted", spaceBaseURLDefaulted),
			zap.Error(err),
		)
		return nil, apperrors.Wrap(apperrors.KindConfig, err)
	}

	userAgent := strings.TrimSpace(config.UserAgent)
	userAgentDefaulted := userAgent == ""
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	logger.Debug("client initialized",
		zap.String("base_url", baseURL),
		zap.Bool("base_url_defaulted", baseURLDefaulted),
		zap.String("space_base_url", spaceBaseURL),
		zap.Bool("space_base_url_defaulted", spaceBaseURLDefaulted),
		zap.Bool("user_agent_defaulted", userAgentDefaulted),
		zap.Bool("csrf_token_present", csrfToken != ""),
	)

	return &Client{
		httpClient:   httpClient,
		baseURL:      baseURL,
		spaceBaseURL: spaceBaseURL,
		cookie:       cookie,
		csrfToken:    csrfToken,
		userAgent:    userAgent,
		referer:      baseURL + "/minutes/home",
		logger:       logger,
	}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultClientTimeout}
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

	return "", apperrors.New(apperrors.KindAuth, "cookie does not contain bv_csrf_token")
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.KindRemote, err)
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
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPRequestFailed(req, duration, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := httpStatusError(req, resp)
		c.logHTTPStatusError(req, resp, duration, 0, err)
		return err
	}

	body := &countingReader{reader: resp.Body}
	var envelope responseEnvelope
	if err := json.NewDecoder(body).Decode(&envelope); err != nil {
		duration = time.Since(start)
		if errors.Is(err, io.EOF) && result == nil {
			c.logHTTPCompleted(req, resp, duration, body.bytesRead)
			return nil
		}
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPJSONDecodeFailed(req, resp, duration, body.bytesRead, err)
		return err
	}

	if envelope.Code != 0 {
		duration = time.Since(start)
		message := envelope.Msg
		if message == "" {
			message = envelope.Message
		}
		if message == "" {
			message = "request failed"
		}
		err := serverCodeError(req, envelope.Code, message)
		c.logHTTPServerCodeError(req, resp, duration, body.bytesRead, envelope.Code, message, err)
		return err
	}

	if result == nil {
		duration = time.Since(start)
		c.logHTTPCompleted(req, resp, duration, body.bytesRead)
		return nil
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		duration = time.Since(start)
		err := apperrors.Errorf(apperrors.KindRemote, "%s %s: response missing data", req.Method, req.URL.RequestURI())
		c.logHTTPJSONDecodeFailed(req, resp, duration, body.bytesRead, err)
		return err
	}

	if err := json.Unmarshal(envelope.Data, result); err != nil {
		duration = time.Since(start)
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPJSONDecodeFailed(req, resp, duration, body.bytesRead, err)
		return err
	}

	duration = time.Since(start)
	c.logHTTPCompleted(req, resp, duration, body.bytesRead)
	return nil
}

func (c *Client) doRaw(req *http.Request) ([]byte, error) {
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPRequestFailed(req, duration, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := httpStatusError(req, resp)
		c.logHTTPStatusError(req, resp, duration, 0, err)
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	duration = time.Since(start)
	if err != nil {
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPReadFailed(req, resp, duration, 0, err)
		return nil, err
	}
	responseSize := int64(len(data))

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
			err := serverCodeError(req, envelope.Code, message)
			c.logHTTPServerCodeError(req, resp, duration, responseSize, envelope.Code, message, err)
			return nil, err
		}
	}

	c.logHTTPCompleted(req, resp, duration, responseSize)
	return data, nil
}

func (c *Client) doStream(req *http.Request, dst io.Writer) error {
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPRequestFailed(req, duration, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := httpStatusError(req, resp)
		c.logHTTPStatusError(req, resp, duration, 0, err)
		return err
	}

	bytesWritten, err := io.Copy(dst, resp.Body)
	duration = time.Since(start)
	if err != nil {
		err = apperrors.Wrap(apperrors.KindRemote, err)
		c.logHTTPReadFailed(req, resp, duration, bytesWritten, err)
		return err
	}

	c.logHTTPCompleted(req, resp, duration, bytesWritten)
	return nil
}

func httpStatusError(req *http.Request, resp *http.Response) error {
	return apperrors.Wrap(apperrors.KindRemote, &HTTPStatusError{
		Method:     req.Method,
		RequestURI: req.URL.RequestURI(),
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
	})
}

func serverCodeError(req *http.Request, code int, message string) error {
	return apperrors.Wrap(apperrors.KindRemote, &ServerCodeError{
		Method:     req.Method,
		RequestURI: req.URL.RequestURI(),
		Code:       code,
		Message:    message,
	})
}

// HTTPStatusError describes a non-2xx HTTP response.
type HTTPStatusError struct {
	Method     string
	RequestURI string
	Status     string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s %s: unexpected HTTP status %s", e.Method, e.RequestURI, e.Status)
}

// ServerCodeError describes a successful HTTP response with a non-zero Feishu
// response code.
type ServerCodeError struct {
	Method     string
	RequestURI string
	Code       int
	Message    string
}

func (e *ServerCodeError) Error() string {
	return fmt.Sprintf("%s %s: server code %d: %s", e.Method, e.RequestURI, e.Code, e.Message)
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (c *Client) logHTTPRequestFailed(req *http.Request, duration time.Duration, err error) {
	c.logger.Debug("http request failed",
		append(c.requestLogFields(req),
			zap.Duration("duration", duration),
			c.errorLogField(req, err),
		)...,
	)
}

func (c *Client) logHTTPStatusError(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64, err error) {
	c.logger.Debug("http status error",
		append(c.responseLogFields(req, resp, duration, responseSize),
			c.errorLogField(req, err),
		)...,
	)
}

func (c *Client) logHTTPJSONDecodeFailed(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64, err error) {
	c.logger.Debug("http json decode failed",
		append(c.responseLogFields(req, resp, duration, responseSize),
			c.errorLogField(req, err),
		)...,
	)
}

func (c *Client) logHTTPReadFailed(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64, err error) {
	c.logger.Debug("http response read failed",
		append(c.responseLogFields(req, resp, duration, responseSize),
			c.errorLogField(req, err),
		)...,
	)
}

func (c *Client) logHTTPServerCodeError(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64, code int, message string, err error) {
	c.logger.Debug("http server code error",
		append(c.responseLogFields(req, resp, duration, responseSize),
			zap.Int("server_code", code),
			zap.String("server_message", message),
			c.errorLogField(req, err),
		)...,
	)
}

func (c *Client) logHTTPCompleted(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64) {
	c.logger.Debug("http request completed",
		c.responseLogFields(req, resp, duration, responseSize)...,
	)
}

func (c *Client) responseLogFields(req *http.Request, resp *http.Response, duration time.Duration, responseSize int64) []zap.Field {
	fields := append(c.requestLogFields(req),
		zap.Int("status", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("content-type")),
		zap.Duration("duration", duration),
		zap.Int64("response_size", responseSize),
	)
	if resp.ContentLength >= 0 {
		fields = append(fields, zap.Int64("content_length", resp.ContentLength))
	}

	return fields
}

func (c *Client) requestLogFields(req *http.Request) []zap.Field {
	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("url_host", req.URL.Host),
		zap.String("url_path", req.URL.Path),
	}
	if req.URL.RawQuery == "" {
		return append(fields, zap.String("url_query", ""))
	}
	if c.shouldLogQuery(req.URL) {
		return append(fields, zap.String("url_query", req.URL.RawQuery))
	}

	query := req.URL.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return append(fields,
		zap.Bool("url_query_redacted", true),
		zap.Bool("url_query_present", true),
		zap.Strings("url_query_keys", keys),
	)
}

func (c *Client) errorLogField(req *http.Request, err error) zap.Field {
	if err == nil {
		return zap.String("error", "")
	}
	if req != nil && c.shouldLogQuery(req.URL) {
		return zap.Error(err)
	}

	return zap.String("error", redactURLQueryText(err.Error()))
}

func (c *Client) shouldLogQuery(rawURL *url.URL) bool {
	for _, base := range []string{c.baseURL, c.spaceBaseURL} {
		parsed, err := url.Parse(base)
		if err == nil && parsed.Host == rawURL.Host {
			return true
		}
	}

	return false
}

func redactURLQueryText(text string) string {
	if !strings.Contains(text, "?") {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text))
	redacting := false
	for _, r := range text {
		if redacting {
			switch r {
			case ' ', '\t', '\n', '\r', '"', '\'', '`':
				redacting = false
				builder.WriteRune(r)
			}
			continue
		}

		if r == '?' {
			builder.WriteString("?<redacted>")
			redacting = true
			continue
		}

		builder.WriteRune(r)
	}

	return builder.String()
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
