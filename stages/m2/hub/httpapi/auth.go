package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) sign(value string) string {
	h := hmac.New(sha256.New, []byte(s.Token))
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Server) authorized(r *http.Request) bool {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(bearer) == len(s.Token) && subtle.ConstantTimeCompare([]byte(bearer), []byte(s.Token)) == 1 {
		return true
	}
	cookie, err := r.Cookie("hf_session")
	if err != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expiry <= time.Now().Unix() || expiry > time.Now().Add(8*time.Hour).Unix() {
		return false
	}
	return hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0])))
}
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" {
		http.Redirect(w, r, "/", 303)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><title>HugoFleet M3 登入</title><h1>本機 Demo 管理登入</h1><p>輸入本次啟動產生的管理 Token。此為 M3 本機 Demo 防護。</p><form method="post" action="/login"><label>管理 Token <input name="token" type="password" autocomplete="off" required maxlength="64"></label><button>登入</button></form></html>`)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil || s.Token == "" || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(s.Token)) != 1 {
		s.fail(w, r, domain.Problem("PERMISSION_DENIED", "invalid Demo management token"))
		return
	}
	expiry := strconv.FormatInt(time.Now().Add(8*time.Hour).Unix(), 10)
	http.SetCookie(w, &http.Cookie{Name: "hf_session", Value: expiry + "." + s.sign(expiry), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	http.Redirect(w, r, "/", 303)
}
