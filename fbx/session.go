package fbx

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type sessionInfo struct {
	sessionToken string
	challenge    string
}

// FreeboxSession represents all the variables used in a session
type FreeboxSession struct {
	client             *FreeboxHttpClient
	getSessionTokenURL string
	getChallengeURL    string

	appToken string

	sessionTokenLastUpdate time.Time
	sessionTokenLock       sync.Mutex
	sessionInfo            *sessionInfo
	oldSessionInfo         *sessionInfo // avoid deleting the sessionInfo too quickly

	logger log.Logger
}

func GetAppToken(client *FreeboxHttpClient, apiVersion *FreeboxAPIVersion, logger log.Logger) (string, error) {
	reqStruct := getFreeboxAuthorize()
	postResponse := struct {
		AppToken string `json:"app_token"`
		TrackID  int64  `json:"track_id"`
	}{}

	url, err := apiVersion.GetURL("login/authorize/")
	if err != nil {
		return "", err
	}

	if err := client.Post(url, reqStruct, &postResponse); err != nil {
		return "", err
	}

	counter := 0
	for {
		counter++
		status := struct {
			Status string `json:"status"`
		}{}

		url, err := apiVersion.GetURL("login/authorize/%d", postResponse.TrackID)
		if err != nil {
			return "", err
		}
		if err := client.Get(url, &status); err != nil {
			return "", err
		}

		switch status.Status {
		case "pending":
			level.Debug(logger).Log("msg", fmt.Sprintf("%s Please accept the login on the Freebox Server"))
			time.Sleep(10 * time.Second)
		case "granted":
			return postResponse.AppToken, nil
		default:
			return "", fmt.Errorf("access is %s", status.Status)
		}
	}
}

func NewFreeboxSession(appToken string, client *FreeboxHttpClient, apiVersion *FreeboxAPIVersion, logger log.Logger) (*FreeboxSession, error) {
	getChallengeURL, err := apiVersion.GetURL("login/")
	if err != nil {
		return nil, err
	}

	getSessionTokenURL, err := apiVersion.GetURL("login/session/")
	if err != nil {
		return nil, err
	}

	result := &FreeboxSession{
		client:             client,
		getSessionTokenURL: getSessionTokenURL,
		getChallengeURL:    getChallengeURL,
		appToken:           appToken,
		logger:             logger,
	}
	if err := result.Refresh(); err != nil {
		return nil, err
	}
	return result, nil
}

func (f *FreeboxSession) IsValid() bool {
	return f.sessionInfo != nil
}

func (f *FreeboxSession) AddHeader(req *http.Request) {
	if f != nil && f.sessionInfo != nil {
		req.Header.Set("X-Fbx-App-Auth", f.sessionInfo.sessionToken)
	}
}

func (f *FreeboxSession) Refresh() error {
	f.sessionTokenLock.Lock()
	defer f.sessionTokenLock.Unlock()

	if sinceLastUpdate := time.Since(f.sessionTokenLastUpdate); sinceLastUpdate < 5*time.Second {
		level.Debug(f.logger).Log("msg", fmt.Sprintf("Updated %v ago. Skipping", sinceLastUpdate))
		return nil
	}

	challenge, err := f.getChallenge()
	if err != nil {
		return err
	}
	sessionToken, err := f.getSessionToken(challenge)
	if err != nil {
		return err
	}
	f.sessionTokenLastUpdate = time.Now()
	f.oldSessionInfo = f.sessionInfo
	f.sessionInfo = &sessionInfo{
		challenge:    challenge,
		sessionToken: sessionToken,
	}
	return nil
}

func (f *FreeboxSession) getChallenge() (string, error) {
	level.Debug(f.logger).Log("msg", fmt.Sprintf("GET challenge: %s", f.getChallengeURL))
	resStruct := struct {
		Challenge string `json:"challenge"`
	}{}

	if err := f.client.Get(f.getChallengeURL, &resStruct); err != nil {
		return "", err
	}

	level.Debug(f.logger).Log("msg", fmt.Sprintf("Challenge: %s", resStruct.Challenge))
	return resStruct.Challenge, nil
}

func (f *FreeboxSession) getSessionToken(challenge string) (string, error) {
	level.Debug(f.logger).Log("msg", fmt.Sprintf("GET SessionToken: %s", f.getSessionTokenURL))
	freeboxAuthorize := getFreeboxAuthorize()

	hash := hmac.New(sha1.New, []byte(f.appToken))
	if _, err := hash.Write([]byte(challenge)); err != nil {
		return "", err
	}
	password := hex.EncodeToString(hash.Sum(nil))

	reqStruct := struct {
		AppID    string `json:"app_id"`
		Password string `json:"password"`
	}{
		AppID:    freeboxAuthorize.AppID,
		Password: password,
	}
	resStruct := struct {
		SessionToken string `json:"session_token"`
	}{}

	if err := f.client.Post(f.getSessionTokenURL, &reqStruct, &resStruct); err != nil {
		return "", err
	}

	level.Debug(f.logger).Log("msg", fmt.Sprintf("SessionToken: %s", resStruct.SessionToken))
	return resStruct.SessionToken, nil
}
