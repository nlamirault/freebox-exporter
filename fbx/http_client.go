package fbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

var (
	errAuthRequired = errors.New("auth_required")
	errInvalidToken = errors.New("invalid_token")
)

type FreeboxHttpClient struct {
	client http.Client
	ctx    context.Context
	logger log.Logger
}

type freeboxAPIResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"msg"`
	ErrorCode string `json:"error_code"`
}

type FreeboxHttpClientCallback func(*http.Request)

func NewFreeboxHttpClient(logger log.Logger) *FreeboxHttpClient {
	result := &FreeboxHttpClient{
		client: http.Client{
			Transport: &http.Transport{
				TLSClientConfig:     newTLSConfig(),
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     10 * time.Minute,
			},
			Timeout: 10 * time.Second,
		},
		ctx:    context.Background(),
		logger: logger,
	}

	return result
}

func (f *FreeboxHttpClient) Get(url string, out interface{}, callbacks ...FreeboxHttpClientCallback) error {
	req, err := http.NewRequestWithContext(f.ctx, "GET", url, nil)

	if err != nil {
		return err
	}
	for _, cb := range callbacks {
		cb(req)
	}
	return f.do(req, out)
}

func (f *FreeboxHttpClient) Post(url string, in interface{}, out interface{}, callbacks ...FreeboxHttpClientCallback) error {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(in); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(f.ctx, "POST", url, buffer)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	for _, cb := range callbacks {
		cb(req)
	}
	return f.do(req, out)
}

func (f *FreeboxHttpClient) do(req *http.Request, out interface{}) error {
	level.Debug(f.logger).Log("msg", fmt.Sprintf("HTTP request: %s %s", req.Method, req.URL.Path))

	res, err := f.client.Do(req)
	if err != nil {
		return err
	}
	var body []byte
	{
		defer res.Body.Close()

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return err
		}
	}
	level.Debug(f.logger).Log("msg", fmt.Sprintf("HTTP Result: %s", string(body)))

	apiResponse := freeboxAPIResponse{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return err
	}
	if !apiResponse.Success {
		switch apiResponse.ErrorCode {
		case errAuthRequired.Error():
			return errAuthRequired
		case errInvalidToken.Error():
			return errInvalidToken
		default:
			return fmt.Errorf("%s %s error_code=%s msg=%s", req.Method, req.URL, apiResponse.ErrorCode, apiResponse.Message)
		}
	}

	result := struct {
		Result interface{} `json:"result"`
	}{
		Result: out,
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	return nil
}
