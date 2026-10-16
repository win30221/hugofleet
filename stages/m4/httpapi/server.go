package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m4/auth"
	"github.com/win30221/hugofleet/stages/m4/web"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/attribute"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ProfileTransport interface {
	service.Transport
	TenantProfile(context.Context, string, string) (domain.TenantProfile, error)
}
type Server struct {
	options   Options
	Auth      *auth.Auth
	Service   *service.Service
	Transport ProfileTransport
	templates *template.Template
}
type sessionKey struct{}

type Options struct {
	Dashboard bool
	Assets    fs.FS
	Metrics   http.Handler
	Traces    func(string) []observability.Record
}

func New(a *auth.Auth, s *service.Service, t ProfileTransport) (*Server, error) {
	return NewWithOptions(a, s, t, Options{})
}
func NewWithOptions(a *auth.Auth, s *service.Service, t ProfileTransport, options Options) (*Server, error) {
	if a == nil || !s.Store.SecurityEnabled() {
		return nil, fmt.Errorf("OIDC session and secured Store required")
	}
	templates, err := template.New("dashboard").Funcs(template.FuncMap{"time": func(ms int64) string {
		if ms == 0 {
			return "尚無"
		}
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}}).ParseFS(web.Assets, "templates/*.html")
	if err == nil && options.Assets != nil {
		templates, err = templates.ParseFS(options.Assets, "templates/*.html")
	}
	return &Server{Auth: a, Service: s, Transport: t, templates: templates, options: options}, err
}
func session(r *http.Request) repository.Session {
	v, _ := r.Context().Value(sessionKey{}).(repository.Session)
	return v
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.Service.Store.Ping(r.Context()) != nil || s.Transport.Ping(r.Context()) != nil {
			s.fail(w, r, domain.Problem("CLUSTER_OFFLINE", "database or Worker channel unavailable"))
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) { s.render(w, r, page{Title: "OIDC 登入", Login: true}) })
	mux.HandleFunc("GET /oidc/login", s.login)
	mux.HandleFunc("GET /oidc/callback", s.callback)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		assets := fs.FS(web.Assets)
		if s.options.Assets != nil {
			assets = s.options.Assets
		}
		css, err := fs.ReadFile(assets, "style.css")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		if _, err = w.Write(css); err != nil {
			slog.Warn("CSS response interrupted", "request_id", domain.ActorContext(r.Context()).RequestID)
		}
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		v := session(r)
		respond(w, 200, map[string]any{"user": v.User, "csrf_token": v.CSRF, "expires_at": v.ExpiresAt})
	})
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Service.Store.VisibleClusters(r.Context())
		s.result(w, r, v, err)
	})
	mux.HandleFunc("GET /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.UsersPage(r.Context(), r.URL.Query().Get("after"), limit)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		pageLink(w, r, v.NextAfter)
		respond(w, 200, v.Items)
	})
	mux.HandleFunc("GET /api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.VisibleTenantsPage(r.Context(), r.URL.Query().Get("after"), limit)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		pageLink(w, r, v.NextAfter)
		respond(w, 200, v.Items)
	})
	mux.HandleFunc("POST /api/v1/tenants", s.activate)
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/quota", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Service.Store.Profiles(r.Context(), r.PathValue("tenant"))
		s.result(w, r, v, err)
	})
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/members", func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.MembersPage(r.Context(), r.PathValue("tenant"), r.URL.Query().Get("after"), limit)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		pageLink(w, r, v.NextAfter)
		respond(w, 200, v.Items)
	})
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/members", s.member)
	auditHandler := func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.AuditEvents(r.Context(), r.URL.Query().Get("tenant"), limit)
		s.result(w, r, v, err)
	}
	mux.HandleFunc("GET /api/v1/audit-events", auditHandler)
	mux.HandleFunc("GET /api/v1/audit", auditHandler)
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/apps", func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.ScopedAppsPage(r.Context(), r.PathValue("tenant"), r.URL.Query().Get("after"), limit)
		s.result(w, r, v, err)
	})
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		limit, err := pageLimit(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, err := s.Service.Store.ScopedDetail(r.Context(), r.PathValue("tenant"), r.PathValue("app"), limit)
		s.result(w, r, v, err)
	})
	mux.HandleFunc("GET /api/v1/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Service.Store.ScopedOperation(r.Context(), r.PathValue("operation"))
		s.result(w, r, v, err)
	})
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps", s.deploy)
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps/{app}/releases", s.deploy)
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps/{app}/rollback", s.rollback)
	mux.HandleFunc("DELETE /api/v1/tenants/{tenant}/apps/{app}", s.delete)
	mux.HandleFunc("GET /{$}", s.index)
	if s.options.Dashboard {
		mux.HandleFunc("GET /clusters", s.index)
		mux.HandleFunc("GET /applications", s.index)
		mux.HandleFunc("GET /tenants", s.index)
		mux.HandleFunc("GET /traces", s.tracePage)
		mux.HandleFunc("GET /api/v1/traces", s.tracePage)
		mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
			if _, err := s.Service.Store.Authorize(r.Context(), "", "PlatformAdmin"); err != nil {
				s.fail(w, r, err)
				return
			}
			if s.options.Metrics == nil {
				s.fail(w, r, domain.Problem("NOT_FOUND", "metrics are disabled"))
				return
			}
			s.options.Metrics.ServeHTTP(w, r)
		})
	}
	mux.HandleFunc("GET /tenants/{tenant}", s.index)
	mux.HandleFunc("GET /tenants/{tenant}/apps/{app}", s.index)
	mux.HandleFunc("POST /ui/tenants", s.formTenant)
	mux.HandleFunc("POST /ui/tenants/{tenant}/members", s.formMember)
	mux.HandleFunc("POST /ui/tenants/{tenant}/apps", s.formApp)
	mux.HandleFunc("POST /ui/tenants/{tenant}/apps/{app}/{action}", s.formApp)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, domain.Problem("NOT_FOUND", "route not found"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		ctx = domain.WithActor(ctx, domain.Actor{RequestID: id})
		if s.options.Dashboard {
			// Only the validated traceparent is accepted from HTTP. User-supplied
			// tracestate and baggage never enter durable operation context.
			ctx = observability.Extract(ctx, r.Header.Get("traceparent"), "")
			var spanCtx context.Context
			spanCtx, span := observability.Tracer().Start(ctx, "hub.http")
			ctx = spanCtx
			span.SetAttributes(attribute.String("request_id", id))
			defer span.End()
			w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			logged := &statusWriter{ResponseWriter: w, status: 200}
			w = logged
			defer func() {
				slog.Info("HTTP request completed", "request_id", id, "method", r.Method, "route", r.Pattern, "status", logged.status, "trace_id", span.SpanContext().TraceID().String())
			}()
		}
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		c := s.Auth.Config
		expectedHost := strings.TrimPrefix(strings.TrimPrefix(c.BaseURL, "https://"), "http://")
		if r.Host != expectedHost {
			s.fail(w, r, domain.Problem("PERMISSION_DENIED", "unexpected management Host"))
			return
		}
		if c.LocalDemo {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			expected := c.HTTPClientIP
			if expected == "" {
				expected = "127.0.0.1"
			}
			if err != nil || ip != expected {
				s.fail(w, r, domain.Problem("PERMISSION_DENIED", "management client is outside the configured loopback/Docker gateway"))
				return
			}
		} else if r.TLS == nil {
			s.fail(w, r, domain.Problem("PERMISSION_DENIED", "HTTPS management connection required"))
			return
		}
		public := r.URL.Path == "/login" || r.URL.Path == "/oidc/login" || r.URL.Path == "/oidc/callback" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/style.css"
		if !public {
			// Preserve a real Origin on browser navigation POSTs. no-referrer
			// turns that Origin into null; cross-origin referrers remain blocked.
			// Public OAuth responses above still use no-referrer.
			w.Header().Set("Referrer-Policy", "same-origin")
			cookie, err := r.Cookie(auth.SessionCookie)
			if err != nil {
				s.unauthenticated(w, r)
				return
			}
			v, err := s.Service.Store.Session(ctx, cookie.Value)
			if err != nil {
				s.unauthenticated(w, r)
				return
			}
			actor := domain.Actor{UserID: v.User.ID, SessionHash: v.Hash, RequestID: id}
			ctx = domain.WithActor(ctx, actor)
			ctx = context.WithValue(ctx, sessionKey{}, v)
			r = r.WithContext(ctx)
			if r.Method != "GET" && r.Method != "HEAD" {
				if r.Header.Get("Origin") != c.BaseURL || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" && r.Header.Get("Sec-Fetch-Site") != "none") {
					s.fail(w, r, domain.Problem("PERMISSION_DENIED", "same-origin mutation required"))
					return
				}
				csrf := r.Header.Get("X-CSRF-Token")
				if strings.Split(r.Header.Get("Content-Type"), ";")[0] == "application/x-www-form-urlencoded" {
					r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
					if err = r.ParseForm(); err != nil {
						s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "invalid or oversized form"))
						return
					}
					csrf = r.PostForm.Get("csrf")
				}
				if !auth.CSRFValid(v.CSRF, csrf) {
					s.fail(w, r, domain.Problem("PERMISSION_DENIED", "CSRF token missing or invalid"))
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.status = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (s *Server) unauthenticated(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.fail(w, r, domain.Problem("UNAUTHENTICATED", "valid platform session required"))
	} else {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	target, binding, err := s.Auth.Begin(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, s.Auth.Cookie(auth.BindingCookie, binding, "/oidc/callback", time.Now().Add(5*time.Minute), http.SameSiteLaxMode))
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	bindingValue := ""
	if binding, err := r.Cookie(auth.BindingCookie); err == nil {
		bindingValue = binding.Value
	}
	http.SetCookie(w, &http.Cookie{Name: auth.BindingCookie, Path: "/oidc/callback", MaxAge: -1, HttpOnly: true, Secure: !s.Auth.Config.LocalDemo, SameSite: http.SameSiteLaxMode})
	v, token, err := s.Auth.Complete(r.Context(), r.URL.Query().Get("state"), bindingValue, r.URL.Query().Get("code"))
	if err != nil {
		if auditErr := s.Service.Store.Audit(r.Context(), "", "LOGIN", "user", "", "DENIED"); auditErr != nil {
			s.fail(w, r, auditErr)
			return
		}
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, s.Auth.Cookie(auth.SessionCookie, token, "/", time.UnixMilli(v.ExpiresAt), http.SameSiteStrictMode))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Service.Store.RevokeSession(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: !s.Auth.Config.LocalDemo, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("HTTP response interrupted")
	}
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	code, message, status := "INTERNAL", "request failed", 500
	var p *domain.Error
	if errors.As(err, &p) {
		code, message = p.Code, p.Message
		switch code {
		case "INVALID_ARGUMENT":
			status = 400
		case "UNAUTHENTICATED":
			status = 401
		case "PERMISSION_DENIED":
			status = 403
		case "NOT_FOUND":
			status = 404
		case "CONFLICT", "OWNERSHIP_CONFLICT":
			status = 409
		case "QUOTA_EXCEEDED":
			status = 422
		case "CLUSTER_OFFLINE", "UNAVAILABLE":
			status = 503
		}
	}
	actor := domain.ActorContext(r.Context())
	slog.Warn("management request rejected", "request_id", actor.RequestID, "actor_id", actor.UserID, "code", code, "method", r.Method, "path", r.URL.Path)
	if actor.UserID != "" && code != "INTERNAL" && r.URL.Path != "/oidc/callback" {
		if e := s.Service.Store.Audit(r.Context(), r.PathValue("tenant"), r.Method, "route", r.URL.Path, "DENIED:"+code); e != nil {
			code, message, status = "INTERNAL", "audit recording failed", 500
		}
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/readyz" || r.URL.Path == "/healthz" {
		respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": actor.RequestID}})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.render(w, r, page{Title: "操作未完成", Error: code + ": " + message})
}
func (s *Server) result(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	respond(w, 200, v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return domain.Problem("INVALID_ARGUMENT", "Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return domain.Problem("INVALID_ARGUMENT", "invalid JSON or unsupported fields")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return domain.Problem("INVALID_ARGUMENT", "one JSON object required")
	}
	return nil
}
func pageLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, domain.Problem("INVALID_ARGUMENT", "limit must be 1..100")
	}
	return n, nil
}

// Array endpoints preserve their response body and expose the cursor in Link.
func pageLink(w http.ResponseWriter, r *http.Request, after string) {
	if after == "" {
		return
	}
	q := url.Values{"after": {after}}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		q.Set("limit", limit)
	}
	w.Header().Set("Link", "<"+r.URL.EscapedPath()+"?"+q.Encode()+">; rel=\"next\"")
}
func (s *Server) accepted(w http.ResponseWriter, r *http.Request, o domain.Accepted, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+o.OperationID)
	respond(w, 202, o)
}
func (s *Server) deploy(w http.ResponseWriter, r *http.Request) {
	var v domain.DeployRequest
	if err := decode(w, r, &v); err != nil {
		s.fail(w, r, err)
		return
	}
	o, err := s.Service.Deploy(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	s.accepted(w, r, o, err)
}
func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	var v domain.RollbackRequest
	if err := decode(w, r, &v); err != nil {
		s.fail(w, r, err)
		return
	}
	o, err := s.Service.Rollback(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	s.accepted(w, r, o, err)
}
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	var v domain.DeleteRequest
	if err := decode(w, r, &v); err != nil {
		s.fail(w, r, err)
		return
	}
	o, err := s.Service.Delete(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	s.accepted(w, r, o, err)
}

type activation struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
	OwnerID   string `json:"owner_id"`
}

func (s *Server) activateTenant(ctx context.Context, v activation) error {
	if _, err := s.Service.Store.Authorize(ctx, "", "PlatformAdmin"); err != nil {
		return err
	}
	p, err := s.Transport.TenantProfile(ctx, v.ClusterID, v.Slug)
	if err != nil {
		var problem *domain.Error
		if errors.As(err, &problem) {
			return err
		}
		return domain.Problem("CLUSTER_OFFLINE", "Worker tenant proof unavailable; bootstrap and verify Agent connection first")
	}
	return s.Service.Store.ActivateTenant(ctx, v.Name, v.OwnerID, p)
}
func (s *Server) activate(w http.ResponseWriter, r *http.Request) {
	var v activation
	if err := decode(w, r, &v); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.activateTenant(r.Context(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	respond(w, 201, map[string]string{"tenant_id": v.Slug, "cluster_id": v.ClusterID, "state": "ACTIVE"})
}

type memberChange struct {
	UserID string  `json:"user_id"`
	Role   *string `json:"role"`
}

func (s *Server) member(w http.ResponseWriter, r *http.Request) {
	var v memberChange
	if err := decode(w, r, &v); err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Role == nil {
		s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "role is required and must be a string; use an empty string to revoke membership"))
		return
	}
	if err := s.Service.Store.SetMember(r.Context(), r.PathValue("tenant"), v.UserID, *v.Role); err != nil {
		s.fail(w, r, err)
		return
	}
	respond(w, 200, map[string]string{"state": "SUCCEEDED"})
}
