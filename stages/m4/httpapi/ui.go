package httpapi

import (
	"fmt"
	"github.com/google/uuid"
	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m2/hub/repository"
	"github.com/win30221/hugofleet/stages/m5/observability"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type page struct {
	View, ClusterFilter, NextURL                                string
	NextTenantsURL, NextUsersURL, NextMembersURL                string
	CurrentTenantOnPage                                         bool
	Overview                                                    *repository.Overview
	AppSpecs                                                    map[string]platform.ApplicationSpec
	Traces                                                      []observability.Record
	Readonly                                                    bool
	Title, Error, RequestID, CSRF, Key, TenantID, Role, Confirm string
	TargetRevision                                              int64
	Login, CanChange, CanOwn                                    bool
	User                                                        domain.User
	Tenants                                                     []domain.Tenant
	Clusters                                                    []domain.Cluster
	Apps                                                        []domain.Application
	Profiles                                                    []domain.TenantProfile
	Members                                                     []domain.Membership
	Users                                                       []domain.User
	Audit                                                       []domain.AuditEvent
	Detail                                                      *domain.Detail
	Accepted                                                    *domain.Operation
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, p page) {
	v := session(r)
	p.User = v.User
	p.CSRF = v.CSRF
	p.RequestID = domain.ActorContext(r.Context()).RequestID
	p.Key = uuid.NewString()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "page", p); err != nil {
		slog.Error("template response interrupted", "request_id", p.RequestID)
	}
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := page{Title: "租戶與應用", TenantID: r.PathValue("tenant")}
	if s.options.Dashboard {
		switch r.URL.Path {
		case "/":
			p.View = "overview"
			p.Title = "平台概覽"
		case "/clusters":
			p.View = "clusters"
			p.Title = "叢集連線"
		case "/applications":
			p.View = "applications"
			p.Title = "應用"
			p.TenantID = r.URL.Query().Get("tenant")
			p.ClusterFilter = r.URL.Query().Get("cluster")
		case "/tenants":
			p.View = "tenants"
			p.Title = "租戶與成員"
		default:
			p.View = "tenant"
		}
	}
	var err error
	tenants, err := s.Service.Store.VisibleTenantsPage(ctx, r.URL.Query().Get("tenants_after"), 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p.Tenants = tenants.Items
	p.NextTenantsURL = directoryPageURL(r, "tenants_after", tenants.NextAfter)
	for _, tenant := range p.Tenants {
		if tenant.ID == p.TenantID {
			p.CurrentTenantOnPage = true
		}
	}
	if p.Clusters, err = s.Service.Store.VisibleClusters(ctx); err != nil {
		s.fail(w, r, err)
		return
	}
	if session(r).User.PlatformAdmin {
		users, err := s.Service.Store.UsersPage(ctx, r.URL.Query().Get("users_after"), 100)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		p.Users = users.Items
		p.NextUsersURL = directoryPageURL(r, "users_after", users.NextAfter)
	}
	if p.TenantID != "" {
		if p.Role, err = s.Service.Store.Authorize(ctx, p.TenantID, "Viewer"); err != nil {
			s.fail(w, r, err)
			return
		}
		p.CanChange = p.Role == "Developer" || p.Role == "Owner" || p.Role == "PlatformAdmin"
		p.CanOwn = p.Role == "Owner" || p.Role == "PlatformAdmin"
		if p.Profiles, err = s.Service.Store.Profiles(ctx, p.TenantID); err != nil {
			s.fail(w, r, err)
			return
		}
		members, err := s.Service.Store.MembersPage(ctx, p.TenantID, r.URL.Query().Get("members_after"), 100)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		p.Members = members.Items
		p.NextMembersURL = directoryPageURL(r, "members_after", members.NextAfter)
		apps, err := s.Service.Store.ScopedAppsPage(ctx, p.TenantID, r.URL.Query().Get("after"), 100)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		p.Apps = apps.Items
		p.NextURL = directoryPageURL(r, "after", apps.NextAfter)
		if p.Audit, err = s.Service.Store.AuditEvents(ctx, p.TenantID, 30); err != nil {
			s.fail(w, r, err)
			return
		}
		if id := r.PathValue("app"); id != "" {
			d, e := s.Service.Store.ScopedDetail(ctx, p.TenantID, id, 100)
			if e != nil {
				s.fail(w, r, e)
				return
			}
			p.Detail = &d
			p.Title = d.Application.Name
			p.Confirm = r.URL.Query().Get("confirm")
			if p.Confirm != "" && p.Confirm != "rollback" && p.Confirm != "delete" {
				s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "unknown confirmation action"))
				return
			}
			if p.Confirm == "delete" && !p.CanOwn {
				s.fail(w, r, domain.Problem("PERMISSION_DENIED", "Owner required to delete"))
				return
			}
			if p.Confirm == "rollback" {
				if !p.CanChange {
					s.fail(w, r, domain.Problem("PERMISSION_DENIED", "Developer required to rollback"))
					return
				}
				p.TargetRevision, err = strconv.ParseInt(r.URL.Query().Get("target"), 10, 64)
				if err != nil || p.TargetRevision < 1 {
					s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "positive rollback source required"))
					return
				}
			}
		}
	}
	if id := r.URL.Query().Get("operation"); id != "" {
		o, e := s.Service.Store.ScopedOperation(ctx, id)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		p.Accepted = &o
	}
	if s.options.Dashboard {
		if p.View == "overview" {
			v, e := s.Service.Store.Overview(ctx)
			if e != nil {
				s.fail(w, r, e)
				return
			}
			p.Overview = &v
		}
		if p.View == "applications" {
			var e error
			p.Apps, e = s.Service.Store.DashboardApps(ctx, p.TenantID, p.ClusterFilter, r.URL.Query().Get("after"), 51)
			if e != nil {
				s.fail(w, r, e)
				return
			}
			p.NextURL = ""
			if len(p.Apps) > 50 {
				p.Apps = p.Apps[:50]
				p.NextURL = directoryPageURL(r, "after", p.Apps[49].ID)
			}
		}
		p.AppSpecs = map[string]platform.ApplicationSpec{}
		for _, a := range p.Apps {
			rel, e := s.Service.Store.Release(ctx, a.ID, a.DesiredRevision)
			if e != nil {
				s.fail(w, r, e)
				return
			}
			p.AppSpecs[a.ID] = rel.Spec
		}
		pending := p.Accepted != nil && p.Accepted.Active()
		if pending && p.Detail != nil {
			p.Readonly = true
			p.CanChange = false
			p.CanOwn = false
		}
		if p.View == "overview" || p.View == "clusters" || (p.View == "applications" && p.TenantID == "") || pending {
			s.refresh(w, r)
		}
	}
	s.render(w, r, p)
}

func directoryPageURL(r *http.Request, key, after string) string {
	if after == "" {
		return ""
	}
	q := r.URL.Query()
	q.Set(key, after)
	q.Del("poll")
	return r.URL.EscapedPath() + "?" + q.Encode()
}

// A maximum of thirty HTML refreshes; editable forms and confirmation pages are
// never refreshed. All repeated requests revalidate session and membership.
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("poll"))
	if n < 0 || n >= 30 || r.URL.Query().Get("confirm") != "" {
		return
	}
	q := url.Values{}
	for _, key := range []string{"tenant", "cluster", "after", "operation", "tenants_after", "users_after", "members_after"} {
		if v := r.URL.Query().Get(key); v != "" {
			q.Set(key, v)
		}
	}
	q.Set("poll", strconv.Itoa(n+1))
	w.Header().Set("Refresh", "5; url="+r.URL.Path+"?"+q.Encode())
}
func (s *Server) tracePage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Service.Store.Authorize(r.Context(), "", "PlatformAdmin"); err != nil {
		s.fail(w, r, err)
		return
	}
	id := strings.ToLower(r.URL.Query().Get("trace"))
	if id != "" {
		if v, err := trace.TraceIDFromHex(id); err != nil || !v.IsValid() {
			s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "valid 32-character trace ID required"))
			return
		}
	}
	items := []observability.Record{}
	if s.options.Traces != nil {
		items = s.options.Traces(id)
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		respond(w, 200, items)
		return
	}
	s.render(w, r, page{Title: "部署追蹤", View: "traces", Traces: items})
}
func (s *Server) formTenant(w http.ResponseWriter, r *http.Request) {
	v := activation{Slug: r.PostForm.Get("slug"), Name: r.PostForm.Get("name"), ClusterID: r.PostForm.Get("cluster_id"), OwnerID: r.PostForm.Get("owner_id")}
	if err := s.activateTenant(r.Context(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/tenants/"+v.Slug, http.StatusSeeOther)
}
func (s *Server) formMember(w http.ResponseWriter, r *http.Request) {
	if err := s.Service.Store.SetMember(r.Context(), r.PathValue("tenant"), r.PostForm.Get("user_id"), r.PostForm.Get("role")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/tenants/"+r.PathValue("tenant"), http.StatusSeeOther)
}
func requiredInt(r *http.Request, key string) (int64, error) {
	n, err := strconv.ParseInt(r.PostForm.Get(key), 10, 64)
	if err != nil {
		return 0, domain.Problem("INVALID_ARGUMENT", "invalid integer "+key)
	}
	return n, nil
}
func (s *Server) formApp(w http.ResponseWriter, r *http.Request) {
	expected, err := requiredInt(r, "expected_desired_revision")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tenant, app, key, action := r.PathValue("tenant"), r.PathValue("app"), r.PostForm.Get("idempotency_key"), r.PathValue("action")
	var accepted domain.Accepted
	switch action {
	case "delete":
		if r.PostForm.Get("confirm") != "delete" {
			s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "delete confirmation required"))
			return
		}
		accepted, err = s.Service.Delete(r.Context(), tenant, app, key, domain.DeleteRequest{ExpectedDesiredRevision: &expected})
	case "rollback":
		if r.PostForm.Get("confirm") != "rollback" {
			s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "rollback confirmation required"))
			return
		}
		target, e := requiredInt(r, "target_revision")
		if e != nil {
			s.fail(w, r, e)
			return
		}
		accepted, err = s.Service.Rollback(r.Context(), tenant, app, key, domain.RollbackRequest{ExpectedDesiredRevision: &expected, TargetRevision: target})
	case "", "releases":
		replicas, e := requiredInt(r, "replicas")
		if e != nil || replicas < 1 || replicas > 3 {
			s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "replicas must be 1..3"))
			return
		}
		port, e := requiredInt(r, "port")
		if e != nil || port < 1 || port > 65535 {
			s.fail(w, r, domain.Problem("INVALID_ARGUMENT", "port must be 1..65535"))
			return
		}
		v := domain.DeployRequest{ExpectedDesiredRevision: &expected, Image: r.PostForm.Get("image"), Replicas: int32(replicas), Port: int32(port), ReadinessPath: r.PostForm.Get("readiness_path"), LivenessPath: r.PostForm.Get("liveness_path"), Resources: platform.ApplicationResources{Requests: platform.ResourceValues{CPU: r.PostForm.Get("request_cpu"), Memory: r.PostForm.Get("request_memory")}, Limits: platform.ResourceValues{CPU: r.PostForm.Get("limit_cpu"), Memory: r.PostForm.Get("limit_memory")}}}
		if app == "" {
			v.Name = r.PostForm.Get("name")
			v.ClusterID = r.PostForm.Get("cluster_id")
		}
		accepted, err = s.Service.Deploy(r.Context(), tenant, app, key, v)
	default:
		err = domain.Problem("NOT_FOUND", "unknown form action")
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tenants/%s/apps/%s?operation=%s", tenant, accepted.ApplicationID, accepted.OperationID), http.StatusSeeOther)
}
