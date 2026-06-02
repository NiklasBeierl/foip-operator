// netcup-auth runs the OAuth2 device flow against the netcup SCP and prints
// the refresh token and user ID needed to create a Kubernetes secret for the
// foip-operator.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	deviceAuthURL = "https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/auth/device"
	tokenURL      = "https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/token"
	userInfoURL   = "https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/userinfo"
	clientID      = "scp"
)

func main() {
	namespace := flag.String("namespace", "", "Kubernetes namespace for the kubectl command")
	secretName := flag.String("secret-name", "netcup-scp-credentials", "Secret name for the kubectl command")
	flag.Parse()

	device, err := startDeviceFlow()
	if err != nil {
		fatal("starting device flow: %v", err)
	}

	fmt.Println("Open this URL in your browser and log in:")
	fmt.Printf("\n  %s\n\n", device.VerificationURIComplete)
	openBrowser(device.VerificationURIComplete)
	fmt.Println("Waiting for you to complete authorization in the browser...")

	tok, err := pollForToken(device.DeviceCode, device.Interval, device.ExpiresIn)
	if err != nil {
		fatal("polling for token: %v", err)
	}

	userID, err := fetchUserID(tok.AccessToken)
	if err != nil {
		fatal("fetching user ID: %v", err)
	}

	fmt.Printf("\nSuccess!\n\n")
	fmt.Printf("User ID:       %s\n", userID)
	fmt.Println("Create the Kubernetes secret with:")
	fmt.Printf("\n  kubectl create secret generic %s \\\n", *secretName)
	if *namespace != "" {
		fmt.Printf("    --namespace=%s \\\n", *namespace)
	}
	fmt.Printf("    --from-literal=userId=%s \\\n", userID)
	fmt.Printf("    --from-literal=refreshToken='%s'\n\n", tok.RefreshToken)
}

// --- device flow ---

type deviceResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func startDeviceFlow() (*deviceResponse, error) {
	resp, err := http.PostForm(deviceAuthURL, url.Values{
		"client_id": {clientID},
		"scope":     {"offline_access openid"},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, data)
	}
	var d deviceResponse
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing device response: %w", err)
	}
	return &d, nil
}

// --- token polling ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

var errAuthPending = errors.New("authorization_pending")

func pollOnce(deviceCode string) (*tokenResponse, error) {
	resp, err := http.PostForm(tokenURL, url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
		"client_id":   {clientID},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode/100 == 2 {
		var t tokenResponse
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("parsing token: %w", err)
		}
		return &t, nil
	}

	var errResp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &errResp)
	switch errResp.Error {
	case "authorization_pending":
		return nil, errAuthPending
	case "slow_down":
		return nil, errAuthPending // caller will add extra delay
	case "access_denied":
		return nil, errors.New("access denied by user")
	case "expired_token":
		return nil, errors.New("device code expired — please run again")
	default:
		return nil, fmt.Errorf("token error %q: %s", errResp.Error, data)
	}
}

func pollForToken(deviceCode string, intervalSec, expiresSec int) (*tokenResponse, error) {
	interval := time.Duration(intervalSec) * time.Second
	deadline := time.Now().Add(time.Duration(expiresSec) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		tok, err := pollOnce(deviceCode)
		if err == nil {
			return tok, nil
		}
		if !errors.Is(err, errAuthPending) {
			return nil, err
		}
	}
	return nil, errors.New("timed out waiting for authorization")
}

// --- user info ---

func fetchUserID(accessToken string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("userinfo HTTP %d: %s", resp.StatusCode, data)
	}
	var info map[string]json.RawMessage
	if err := json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("parsing userinfo: %w", err)
	}
	raw, ok := info["id"]
	if !ok {
		// some Keycloak configs use "sub" instead
		raw, ok = info["sub"]
		if !ok {
			return "", errors.New(`userinfo response has no "id" or "sub" field`)
		}
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", fmt.Errorf("parsing user ID: %w", err)
	}
	return id, nil
}

// --- helpers ---

func openBrowser(u string) {
	var cmd string
	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "darwin":
		cmd = "open"
	default:
		return
	}
	if err := exec.Command(cmd, u).Start(); err == nil {
		fmt.Println("(opened in browser)")
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
