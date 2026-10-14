package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/service"
	"github.com/win30221/hugofleet/stages/m2/web"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Token         string
	AllowedHTTPIP string
	Service       *service.Service
	Host          string
	transport     service.Transport
	templates     *template.Template
}

func New(s *service.Service, t service.Transport, host string) (*Server, error) {
	templates, err := template.New("hub").Funcs(template.FuncMap{"time": func(ms int64) string {
		if ms == 0 {
			return "尚無"
		}
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}}).ParseFS(web.Assets, "templates/*.html")
	return &Server{Service: s, Host: host, transport: t, templates: templates}, err
}

type requestIDKey struct{}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if s.Service.Store.Ping(ctx) != nil || s.transport.Ping(ctx) != nil {
			s.fail(w, r, domain.Problem("CLUSTER_OFFLINE", "database or local cluster unavailable"))
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Store.Clusters(r.Context())
		s.result(w, r, v, e)
	})
	mux.HandleFunc("GET /api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Store.Tenants(r.Context())
		s.result(w, r, v, e)
	})
	mux.HandleFunc("POST /api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, domain.Problem("PERMISSION_DENIED", "M2 uses the bootstrapped demo tenant; tenant provisioning requires M4"))
	})
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/apps", s.list)
	mux.HandleFunc("GET /api/v1/tenants/{tenant}/apps/{app}", func(w http.ResponseWriter, r *http.Request) {
		n, e := pageLimit(r, 50)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		d, e := s.Service.Store.Detail(r.Context(), r.PathValue("tenant"), r.PathValue("app"), n)
		s.result(w, r, d, e)
	})
	mux.HandleFunc("GET /api/v1/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Service.Store.Operation(r.Context(), r.PathValue("operation"))
		s.result(w, r, v, e)
	})
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps", s.deploy)
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps/{app}/releases", s.deploy)
	mux.HandleFunc("POST /api/v1/tenants/{tenant}/apps/{app}/rollback", s.rollback)
	mux.HandleFunc("DELETE /api/v1/tenants/{tenant}/apps/{app}", s.delete)
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /apps/{app}", s.index)
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		css, e := web.Assets.ReadFile("style.css")
		if e != nil {
			s.fail(w, r, e)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(css)
	})
	mux.HandleFunc("POST /ui/apps", s.form)
	mux.HandleFunc("POST /ui/apps/{app}/{action}", s.form)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, domain.Problem("NOT_FOUND", "route not found"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Host != s.Host {
			s.fail(w, r, domain.Problem("PERMISSION_DENIED", "unexpected Host"))
			return
		}
		if s.AllowedHTTPIP != "" {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || ip != s.AllowedHTTPIP {
				slog.Warn("management network peer rejected", "client_ip", ip, "expected_ip", s.AllowedHTTPIP)
				s.fail(w, r, domain.Problem("PERMISSION_DENIED", "management client is outside the local Docker gateway"))
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			site := r.Header.Get("Sec-Fetch-Site")
			if (origin != "" && origin != "http://"+s.Host) || (site != "" && site != "same-origin" && site != "none") {
				s.fail(w, r, domain.Problem("PERMISSION_DENIED", "cross-origin management request rejected"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		if s.Token != "" && r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && r.URL.Path != "/login" && !s.authorized(r) {
			cancel()
			if strings.HasPrefix(r.URL.Path, "/api/") {
				s.fail(w, r, domain.Problem("PERMISSION_DENIED", "Demo management token required"))
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("HTTP response write failed", "error", err)
	}
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := 500
	code, message := "INTERNAL", "request failed"
	var p *domain.Error
	if errors.As(err, &p) {
		code, message = p.Code, p.Message
		switch code {
		case "INVALID_ARGUMENT":
			status = 400
		case "NOT_FOUND":
			status = 404
		case "CONFLICT":
			status = 409
		case "QUOTA_EXCEEDED":
			status = 422
		case "PERMISSION_DENIED":
			status = 403
		case "CLUSTER_OFFLINE":
			status = 503
		}
	} else {
		slog.Error("HTTP request failed", "request_id", r.Context().Value(requestIDKey{}), "error", err)
	}
	respond(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": r.Context().Value(requestIDKey{})}})
}
func (s *Server) result(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		s.fail(w, r, e)
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
	if err := d.Decode(v); err != nil {
		return domain.Problem("INVALID_ARGUMENT", "invalid JSON or unsupported fields")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return domain.Problem("INVALID_ARGUMENT", "body must contain one JSON object")
	}
	return nil
}
func (s *Server) deploy(w http.ResponseWriter, r *http.Request) {
	var v domain.DeployRequest
	if e := decode(w, r, &v); e != nil {
		s.fail(w, r, e)
		return
	}
	o, e := s.Service.Deploy(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	if e != nil {
		s.fail(w, r, e)
		return
	}
	respond(w, 202, o)
}
func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	var v domain.RollbackRequest
	if e := decode(w, r, &v); e != nil {
		s.fail(w, r, e)
		return
	}
	o, e := s.Service.Rollback(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	if e != nil {
		s.fail(w, r, e)
		return
	}
	respond(w, 202, o)
}
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	var v domain.DeleteRequest
	if e := decode(w, r, &v); e != nil {
		s.fail(w, r, e)
		return
	}
	o, e := s.Service.Delete(r.Context(), r.PathValue("tenant"), r.PathValue("app"), r.Header.Get("Idempotency-Key"), v)
	if e != nil {
		s.fail(w, r, e)
		return
	}
	respond(w, 202, o)
}
func pageLimit(r *http.Request, fallback int) (int, error) {
	if r.URL.Query().Get("limit") == "" {
		return fallback, nil
	}
	v, e := strconv.Atoi(r.URL.Query().Get("limit"))
	if e != nil || v < 1 || v > 100 {
		return 0, domain.Problem("INVALID_ARGUMENT", "limit must be from 1 to 100")
	}
	return v, nil
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("tenant") != domain.TenantID {
		s.fail(w, r, domain.Problem("NOT_FOUND", "tenant not found"))
		return
	}
	n, e := pageLimit(r, 50)
	if e != nil {
		s.fail(w, r, e)
		return
	}
	v, e := s.Service.Store.Apps(r.Context(), domain.TenantID, r.URL.Query().Get("after"), n+1)
	if e != nil {
		s.fail(w, r, e)
		return
	}
	next := ""
	if len(v) > n {
		next = v[n-1].ID
		v = v[:n]
	}
	respond(w, 200, map[string]any{"items": v, "next_after": next})
}

type page struct {
	Mode                                                                        string
	Detail                                                                      *domain.Detail
	Apps                                                                        []domain.Application
	Clusters                                                                    []domain.Cluster
	CSRF, CreateKey, ReleaseKey, RollbackKey, DeleteKey, Error, Next, RequestID string
	Refresh                                                                     bool
	Image                                                                       string
	Replicas, Port                                                              int32
	Resources                                                                   platform.ApplicationResources
	ReadinessPath, LivenessPath                                                 string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) { s.render(w, r, 200, "") }
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, message string) {
	csrf := ""
	cookie, e := r.Cookie("hf_csrf")
	if e == nil && len(cookie.Value) == 36 {
		csrf = cookie.Value
	} else {
		csrf = uuid.NewString()
		http.SetCookie(w, &http.Cookie{Name: "hf_csrf", Value: csrf, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	p := page{CSRF: csrf, CreateKey: uuid.NewString(), ReleaseKey: uuid.NewString(), RollbackKey: uuid.NewString(), DeleteKey: uuid.NewString(), Error: message, Image: "pricing-api:v1", Replicas: 1, Port: 8080, Resources: s.Service.Policy.DefaultResources, ReadinessPath: "/readyz", LivenessPath: "/healthz"}
	p.RequestID, _ = r.Context().Value(requestIDKey{}).(string)
	p.Mode = "M2"
	if s.Token != "" {
		p.Mode = "M3"
	}
	if id := r.PathValue("app"); id != "" {
		d, e := s.Service.Store.Detail(r.Context(), domain.TenantID, id, 50)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		p.Detail = &d
		for _, operation := range d.Operations {
			if operation.Active() {
				p.Refresh = true
				break
			}
		}
		if len(d.Releases) > 0 {
			v := d.Releases[0].Spec
			p.Image = v.Image
			p.Replicas = v.Replicas
			p.Port = v.Port
			p.Resources = v.Resources
			p.ReadinessPath = v.ReadinessPath
			p.LivenessPath = v.LivenessPath
		}
	} else {
		p.Clusters, e = s.Service.Store.Clusters(r.Context())
		if e != nil {
			s.fail(w, r, e)
			return
		}
		p.Apps, e = s.Service.Store.Apps(r.Context(), domain.TenantID, r.URL.Query().Get("after"), 51)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		if len(p.Apps) > 50 {
			p.Next = p.Apps[49].ID
			p.Apps = p.Apps[:50]
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if e = s.templates.ExecuteTemplate(w, "page", p); e != nil {
		slog.Error("UI template failed", "error", e)
	}
}
func (s *Server) form(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if e := r.ParseForm(); e != nil {
		s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "invalid form"))
		return
	}
	cookie, e := r.Cookie("hf_csrf")
	if e != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.PostForm.Get("csrf"))) != 1 {
		s.fail(w, r, domain.Problem("PERMISSION_DENIED", "invalid CSRF token"))
		return
	}
	expected, e := strconv.ParseInt(r.PostForm.Get("expected_desired_revision"), 10, 64)
	if e != nil {
		s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "expected_desired_revision is required"))
		return
	}
	key := r.PostForm.Get("idempotency_key")
	if (r.PathValue("action") == "rollback" || r.PathValue("action") == "delete") && r.PostForm.Get("confirm") != "yes" {
		s.render(w, r, 400, "請先勾選確認，再執行回滾或刪除")
		return
	}
	app := r.PathValue("app")
	var o domain.Accepted
	var err error
	switch r.PathValue("action") {
	case "rollback":
		target, e := strconv.ParseInt(r.PostForm.Get("target_revision"), 10, 64)
		if e != nil {
			err = domain.Problem("INVALID_ARGUMENT", "invalid target revision")
		} else {
			o, err = s.Service.Rollback(r.Context(), domain.TenantID, app, key, domain.RollbackRequest{TargetRevision: target, ExpectedDesiredRevision: &expected})
		}
	case "delete":
		o, err = s.Service.Delete(r.Context(), domain.TenantID, app, key, domain.DeleteRequest{ExpectedDesiredRevision: &expected})
	case "", "releases":
		replicas, e1 := strconv.ParseInt(r.PostForm.Get("replicas"), 10, 32)
		port, e2 := strconv.ParseInt(r.PostForm.Get("port"), 10, 32)
		if e1 != nil || e2 != nil {
			err = domain.Problem("INVALID_ARGUMENT", "replicas and port must be integers")
			break
		}
		v := domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: r.PostForm.Get("image"), Replicas: int32(replicas), Port: int32(port), ReadinessPath: r.PostForm.Get("readiness_path"), LivenessPath: r.PostForm.Get("liveness_path"), Resources: platform.ApplicationResources{Requests: platform.ResourceValues{CPU: r.PostForm.Get("request_cpu"), Memory: r.PostForm.Get("request_memory")}, Limits: platform.ResourceValues{CPU: r.PostForm.Get("limit_cpu"), Memory: r.PostForm.Get("limit_memory")}}}
		if app == "" {
			v.Name = r.PostForm.Get("name")
			v.ClusterID = r.PostForm.Get("cluster_id")
			if v.ClusterID == "" && s.Token == "" {
				v.ClusterID = domain.ClusterID
			}
		}
		o, err = s.Service.Deploy(r.Context(), domain.TenantID, app, key, v)
	default:
		err = domain.Problem("NOT_FOUND", "action not found")
	}
	if err != nil {
		status := 400
		var p *domain.Error
		if errors.As(err, &p) {
			switch p.Code {
			case "CONFLICT":
				status = 409
			case "QUOTA_EXCEEDED":
				status = 422
			case "CLUSTER_OFFLINE":
				status = 503
			case "NOT_FOUND":
				status = 404
			case "PERMISSION_DENIED":
				status = 403
			}
		} else {
			slog.Error("UI operation failed", "request_id", r.Context().Value(requestIDKey{}), "error", err)
			status = 500
			err = errors.New("request failed")
		}
		s.render(w, r, status, fmt.Sprint(err))
		return
	}
	http.Redirect(w, r, "/apps/"+o.ApplicationID, http.StatusSeeOther)
}
