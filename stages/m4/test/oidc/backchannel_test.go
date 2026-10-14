//go:build oidcintegration

package oidctest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/testdriver"
)

// Uses the running real Dex, while keeping login transactions and sessions in
// an independent SQLite DB. It never calls or changes the running Hub callback.
func TestRealDexCanonicalBackchannel(t *testing.T) {
	state := os.Getenv("HF_M4_STATE")
	settings, err := testdriver.ReadSettings(filepath.Join(state, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(state, "subjects.json"))
	if err != nil {
		t.Fatal("verified private subjects unavailable")
	}
	var subjects map[string]string
	err = json.NewDecoder(io.LimitReader(f, 16<<10)).Decode(&subjects)
	closeErr := f.Close()
	if err != nil || closeErr != nil || subjects["admin"] == "" || subjects["developer-a"] == "" {
		t.Fatal("verified private subjects invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.ConfigureSecurity(ctx, []string{"hugofleet-a", "hugofleet-b"}, settings.OIDC.Issuer, subjects["admin"]); err != nil {
		t.Fatal(err)
	}
	a, err := auth.New(ctx, settings.OIDC, store)
	if err != nil {
		t.Fatal(err)
	}
	target, binding, err := a.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := testdriver.Account(settings, "developer-a")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := testdriver.FollowDex(ctx, testdriver.BrowserClient(), settings.OIDC, target, account)
	if err != nil {
		t.Fatal(err)
	}
	query := callback.Query()
	session, opaque, err := a.Complete(ctx, query.Get("state"), binding, query.Get("code"))
	if err != nil {
		t.Fatal(err)
	}
	if session.User.Issuer != settings.OIDC.Issuer || session.User.Subject != subjects["developer-a"] || session.User.PlatformAdmin || len(opaque) != 43 || len(session.CSRF) != 43 {
		t.Fatal("real verified identity, untrusted groups or opaque session boundary invalid")
	}
	loaded, err := store.Session(ctx, opaque)
	if err != nil || loaded.User.ID != session.User.ID || loaded.Hash != repository.SecretHash(opaque) {
		t.Fatal("real verified session was not durably persisted")
	}
	if _, _, err = a.Complete(ctx, query.Get("state"), binding, query.Get("code")); err == nil {
		t.Fatal("real Code/state replay accepted")
	}
	var problem *domain.Error
	if !errors.As(err, &problem) || problem.Code != "UNAUTHENTICATED" {
		t.Fatal("Code/state replay failed for an unexpected reason")
	}
	t.Log("PASS canonical discovery/token/JWKS with real Dex Code/S256/nonce/signature, immutable issuer/sub, opaque durable session and one-use state; elevated groups ignored")
}
