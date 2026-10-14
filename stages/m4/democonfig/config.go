// Package democonfig generates disposable, private local Dex credentials. These
// static accounts are real OIDC test identities, not production IAM or mocks.
package democonfig

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"time"
)

type Account struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	ID       string `json:"id"`
}
type Settings struct {
	OIDC     auth.Config `json:"oidc"`
	Accounts []Account   `json:"accounts"`
}

func WritePrivate(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}
func Generate(dir, issuer, baseURL string) error {
	c := auth.Config{Issuer: issuer, BaseURL: baseURL, ClientID: "hugofleet", ClientSecret: repository.RandomSecret(), LocalDemo: true, SessionTTL: time.Hour}
	if err := c.Validate(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "accounts.json")); err == nil {
		return fmt.Errorf("Demo credentials already exist; refusing silent replacement")
	}
	if err := os.MkdirAll(filepath.Join(dir, "dex"), 0700); err != nil {
		return err
	}
	settings := Settings{OIDC: c}
	passwords := []map[string]any{}
	for _, name := range []string{"admin", "owner-a", "developer-a", "owner-b", "viewer-a", "outsider"} {
		account := Account{Name: name, Email: name + "@hugofleet.example.test", Password: repository.RandomSecret(), ID: uuid.NewString()}
		hash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		settings.Accounts = append(settings.Accounts, account)
		passwords = append(passwords, map[string]any{"email": account.Email, "hash": string(hash), "username": name, "userID": account.ID, "groups": []string{"PlatformAdmin", "Owner"}})
	}
	// All accounts deliberately carry elevated groups. HugoFleet must ignore them
	// and use only its immutable subject binding and DB memberships.
	dex := map[string]any{"issuer": issuer, "storage": map[string]any{"type": "memory"}, "web": map[string]string{"http": "0.0.0.0:5556"}, "logger": map[string]any{"level": "warn", "format": "json", "excludeFields": []string{"email", "username", "preferred_username", "groups"}}, "enablePasswordDB": true, "oauth2": map[string]any{"responseTypes": []string{"code"}, "grantTypes": []string{"authorization_code"}, "skipApprovalScreen": true, "pkce": map[string]any{"enforce": true, "codeChallengeMethodsSupported": []string{"S256"}}}, "expiry": map[string]string{"idTokens": "15m"}, "staticClients": []map[string]any{{"id": c.ClientID, "secret": c.ClientSecret, "name": "HugoFleet local Demo", "redirectURIs": []string{baseURL + "/oidc/callback"}}}, "staticPasswords": passwords}
	dex["frontend"] = map[string]string{"dir": "/srv/dex/web", "issuer": "HugoFleet local Demo"}
	if err := WritePrivate(filepath.Join(dir, "dex", "config.yaml"), dex); err != nil {
		return err
	}
	return WritePrivate(filepath.Join(dir, "accounts.json"), settings)
}
