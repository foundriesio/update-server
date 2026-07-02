// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/foundriesio/update-server/cli/config"
	models "github.com/foundriesio/update-server/server/ui/api"
)

var LoginCmd = &cobra.Command{
	Use:   "login <context-name> <server-url>",
	Short: "Configure authentication for a server",
	Long: `Login to a Foundries Update Server by configuring a context with authentication.

This command will guide you through the authentication process and save
the configuration to ~/.config/satcli.yaml.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		contextName := args[0]
		serverURL := args[1]

		token, _ := cmd.Flags().GetString("token")
		setDefault, _ := cmd.Flags().GetBool("set-default")
		configPath, _ := cmd.Flags().GetString("config")
		scopes, _ := cmd.Flags().GetString("scopes")
		expiresInDays, _ := cmd.Flags().GetInt("expires-in-days")

		cobra.CheckErr(login(configPath, contextName, serverURL, token, scopes, expiresInDays, setDefault))
	},
}

func init() {
	LoginCmd.Flags().String("token", "", "API token for authentication (skips OAuth2 device flow)")
	LoginCmd.Flags().Bool("set-default", true, "Set this context as the default")
	LoginCmd.Flags().String("config", "", "Specify the configuration file to use")
	LoginCmd.Flags().String("scopes", "devices:read-update,updates:read-update", "Comma-separated list of OAuth2 scopes to request (optional)")
	LoginCmd.Flags().Int("expires-in-days", 90, "Number of days until the access token expires")
}

func login(configPath, contextName, serverURL, token, scopes string, expiresInDays int, setDefault bool) error {
	if token != "" {
		return saveToken(configPath, contextName, serverURL, token, setDefault)
	}

	fmt.Println("Initiating OAuth2 device authorization flow...")
	expires := time.Now().Add(time.Duration(expiresInDays) * 24 * time.Hour).Unix()
	return oauth2DeviceFlow(configPath, contextName, serverURL, scopes, expires, setDefault)
}

func saveToken(configPath, contextName, serverURL, token string, setDefault bool) error {
	// Load existing config or create new one
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = &config.Config{
				Contexts: make(map[string]config.Context),
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]config.Context)
	}
	cfg.Contexts[contextName] = config.Context{
		URL:   serverURL,
		Token: token,
	}

	if setDefault {
		cfg.ActiveContext = contextName
	}

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Successfully configured context '%s'\n", contextName)
	fmt.Printf("  Server URL: %s\n", serverURL)
	if setDefault {
		fmt.Printf("  Set as default context\n")
	}

	return nil
}

type oauth2Error struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

func (e *oauth2Error) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("%s: %s", e.ErrorCode, e.ErrorDescription)
	}
	return e.ErrorCode
}

func oauth2DeviceFlow(configPath, contextName, serverURL, scopes string, expires int64, setDefault bool) error {
	// Step 1: Request device code
	codeReq := models.DeviceCodeRequest{
		Scopes:       scopes,
		TokenExpires: expires,
	}
	jsonData, err := json.Marshal(codeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(serverURL+"/oauth2/device/code", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close() // nolint:errcheck

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get device code (status %d): %s", resp.StatusCode, string(body))
	}

	var codeResp models.DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&codeResp); err != nil {
		return fmt.Errorf("failed to decode device code response: %w", err)
	}

	// Step 2: Display user code and verification URI
	fmt.Println()
	fmt.Println("------------------------------------------------")
	fmt.Printf("  Visit: %s\n", codeResp.VerificationURI)
	fmt.Println()
	fmt.Printf("  Enter code: %s\n", codeResp.UserCode)
	fmt.Println("------------------------------------------------")
	fmt.Println()
	fmt.Println("Waiting for authorization...")

	// Step 3: Poll for token
	pollInterval := time.Duration(codeResp.Interval) * time.Second
	expiresAt := time.Now().Add(time.Duration(codeResp.ExpiresIn) * time.Second)

	for time.Now().Before(expiresAt) {
		time.Sleep(pollInterval)

		token, err := pollForToken(serverURL, codeResp.DeviceCode)
		if err == nil {
			// Success! Save the token
			fmt.Println()
			fmt.Println("✓ Authorization successful!")
			return saveToken(configPath, contextName, serverURL, token, setDefault)
		}

		// Check if we should continue polling
		if oauth2Err, ok := err.(*oauth2Error); ok {
			switch oauth2Err.ErrorCode {
			case "authorization_pending":
				continue
			case "slow_down":
				pollInterval *= 2
				continue
			case "access_denied":
				return fmt.Errorf("authorization was denied")
			case "expired_token":
				return fmt.Errorf("authorization code expired")
			default:
				return fmt.Errorf("OAuth2 error: %s - %s", oauth2Err.ErrorCode, oauth2Err.ErrorDescription)
			}
		}

		return fmt.Errorf("failed to get token: %w", err)
	}

	return fmt.Errorf("authorization timed out")
}

func pollForToken(serverURL, deviceCode string) (string, error) {
	tokenReq := models.DeviceTokenRequest{
		DeviceCode: deviceCode,
		GrantType:  "urn:ietf:params:oauth:grant-type:device_code",
	}

	jsonData, err := json.Marshal(tokenReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(serverURL+"/oauth2/device/token", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == 200 {
		var tokenResp models.DeviceTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return "", fmt.Errorf("failed to decode token response: %w", err)
		}
		return tokenResp.AccessToken, nil
	}

	var errResp oauth2Error
	if err := json.Unmarshal(body, &errResp); err != nil {
		return "", fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(body))
	}

	return "", &errResp
}
