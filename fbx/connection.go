package fbx

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/go-kit/log"
)

type config struct {
	APIVersion *FreeboxAPIVersion `json:"api"`
	AppToken   string             `json:"app_token"`
}

type FreeboxConnection struct {
	client  *FreeboxHttpClient
	session *FreeboxSession
	config  config
	logger  log.Logger
}

/*
 * FreeboxConnection
 */

func NewFreeboxConnectionFromServiceDiscovery(discovery FreeboxDiscovery, forceApiVersion int, logger log.Logger) (*FreeboxConnection, error) {
	client := NewFreeboxHttpClient(logger)
	apiVersion, err := NewFreeboxAPIVersion(client, discovery, forceApiVersion, logger)
	if err != nil {
		return nil, err
	}
	appToken, err := GetAppToken(client, apiVersion, logger)
	if err != nil {
		return nil, err
	}
	session, err := NewFreeboxSession(appToken, client, apiVersion, logger)
	if err != nil {
		return nil, err
	}

	return &FreeboxConnection{
		client:  client,
		session: session,
		config: config{
			APIVersion: apiVersion,
			AppToken:   appToken,
		},
		logger: logger,
	}, nil
}

func NewFreeboxConnectionFromConfig(reader io.Reader, forceApiVersion int, logger log.Logger) (*FreeboxConnection, error) {
	client := NewFreeboxHttpClient(logger)
	config := config{}
	if err := json.NewDecoder(reader).Decode(&config); err != nil {
		return nil, err
	}
	if err := config.APIVersion.setQueryApiVersion(forceApiVersion); err != nil {
		return nil, err
	}
	if !config.APIVersion.IsValid() {
		return nil, fmt.Errorf("invalid api_version: %v", config.APIVersion)
	}
	if config.AppToken == "" {
		return nil, fmt.Errorf("invalid app_token: %s", config.AppToken)
	}

	session, err := NewFreeboxSession(config.AppToken, client, config.APIVersion, logger)
	if err != nil {
		return nil, err
	}

	return &FreeboxConnection{
		client:  client,
		session: session,
		config:  config,
	}, nil
}

func (f *FreeboxConnection) WriteConfig(writer io.Writer) error {
	return json.NewEncoder(writer).Encode(&f.config)
}

func (f *FreeboxConnection) get(path string, out interface{}) error {
	return f.getInternal(path, out, false)
}

func (f *FreeboxConnection) getInternal(path string, out interface{}, retry bool) error {
	url, err := f.config.APIVersion.GetURL(path)
	if err != nil {
		return err
	}

	if err := f.client.Get(url, out, f.session.AddHeader); err != nil {
		if retry {
			return err
		}

		switch err {
		case errAuthRequired, errInvalidToken:
			err := f.session.Refresh()
			if err != nil {
				return err
			}
			return f.getInternal(path, out, true)
		default:
			return err
		}
	}

	return nil
}

func (f *FreeboxConnection) Close() error {
	url, err := f.config.APIVersion.GetURL("login/logout/")
	if err != nil {
		return err
	}
	return f.client.Post(url, nil, nil, f.session.AddHeader)
}
