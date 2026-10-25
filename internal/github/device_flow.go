package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zhongtait/gh-account/internal/config"
)

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type userResponse struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// maxPollRetries bounds transient network retries so an offline machine
// fails fast instead of spinning until the device code expires.
const maxPollRetries = 5

var waitForPoll = wait

func (c *OAuthClient) requestDeviceCode(ctx context.Context, hostname string) (deviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("scope", defaultScope)
	body, status, err := c.doForm(ctx, oauthURL(hostname, "/login/device/code"), form)
	if err != nil {
		return deviceCodeResponse{}, err
	}
	if status < 200 || status >= 300 {
		return deviceCodeResponse{}, fmt.Errorf("GitHub returned HTTP %d: %s", status, responseMessage(body))
	}
	var response deviceCodeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return deviceCodeResponse{}, fmt.Errorf("decode device response: %w", err)
		}
		response.DeviceCode = values.Get("device_code")
		response.UserCode = values.Get("user_code")
		response.VerificationURI = values.Get("verification_uri")
		response.VerificationURIComplete = values.Get("verification_uri_complete")
		response.ExpiresIn, _ = strconv.Atoi(values.Get("expires_in"))
		response.Interval, _ = strconv.Atoi(values.Get("interval"))
	}
	if response.DeviceCode == "" || response.UserCode == "" || response.VerificationURI == "" {
		return deviceCodeResponse{}, errors.New("GitHub returned an incomplete device code response")
	}
	if response.Interval <= 0 {
		response.Interval = 5
	}
	return response, nil
}

type accessToken struct {
	AccessToken string
	TokenType   string
	Scope       string
	Login       string
}

func (c *OAuthClient) pollToken(ctx context.Context, hostname string, device deviceCodeResponse) (accessToken, error) {
	interval := time.Duration(device.Interval) * time.Second
	deadline := time.Duration(device.ExpiresIn) * time.Second
	if deadline <= 0 {
		deadline = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	retries := 0
	for {
		// GitHub's interval is a minimum delay between access-token polls;
		// waiting before the first request avoids an immediate rate-limit hit.
		if err := waitForPoll(ctx, interval); err != nil {
			return accessToken{}, err
		}
		if ctx.Err() != nil {
			return accessToken{}, ctx.Err()
		}
		form := url.Values{}
		form.Set("client_id", c.ClientID)
		form.Set("device_code", device.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		body, status, err := c.doForm(ctx, oauthURL(hostname, "/login/oauth/access_token"), form)
		if err != nil {
			// Transient transport failures (offline Wi-Fi, DNS hiccup) should
			// not abort the whole device flow: keep polling until the code
			// expires or the user cancels.
			if ctx.Err() != nil {
				return accessToken{}, ctx.Err()
			}
			if retries >= maxPollRetries || !isTransientNetErr(err) {
				return accessToken{}, err
			}
			retries++
			continue
		}
		var response tokenResponse
		if jsonErr := json.Unmarshal(body, &response); jsonErr != nil {
			values, parseErr := url.ParseQuery(string(body))
			if parseErr != nil {
				return accessToken{}, fmt.Errorf("decode token response: %w", jsonErr)
			}
			response.AccessToken = values.Get("access_token")
			response.TokenType = values.Get("token_type")
			response.Scope = values.Get("scope")
			response.Error = values.Get("error")
			response.Description = values.Get("error_description")
		}
		if status < 200 || status >= 300 {
			return accessToken{}, fmt.Errorf("GitHub returned HTTP %d: %s", status, responseMessage(body))
		}
		if response.AccessToken != "" {
			credential := config.Credential{Hostname: hostname, AccessToken: response.AccessToken, TokenType: response.TokenType, Scope: response.Scope}
			user, userErr := c.getUser(ctx, credential)
			if userErr != nil {
				return accessToken{}, fmt.Errorf("read authenticated GitHub user: %w", userErr)
			}
			return accessToken{AccessToken: response.AccessToken, TokenType: response.TokenType, Scope: response.Scope, Login: user.Login}, nil
		}
		switch response.Error {
		case "authorization_pending", "":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "expired_token":
			return accessToken{}, errors.New("device code expired")
		case "access_denied":
			return accessToken{}, errors.New("GitHub authorization was denied")
		default:
			message := response.Description
			if message == "" {
				message = response.Error
			}
			return accessToken{}, fmt.Errorf("GitHub OAuth error: %s", message)
		}
	}
}

// isTransientNetErr reports whether err looks like a transient network or
// transport failure that is worth retrying while the device code is still
// valid. DNS hiccups, connection resets and timeouts are common on flaky
// links; aborting immediately would force the user to restart login.
func isTransientNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true // includes *url.Error timeouts from http.Client
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "connection reset", "eof", "broken pipe", "no such host", "temporary failure", "tls:", "proxy"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
