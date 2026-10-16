//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"github.com/win30221/hugofleet/stages/m4/bootstrap"
	"golang.org/x/net/html"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// submitForm uses the real rendered CSRF, idempotency key and expected revision.
// It cannot replace those hidden fields or submit to an external destination.
func (s *suite) submitForm(who, page, action string, overrides map[string]string) domain.Accepted {
	s.t.Helper()
	raw := s.api(who, "GET", page, "", nil, 200, nil)
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		s.t.Fatal(err)
	}
	attr := func(n *html.Node, key string) string {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	var formNode *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" && attr(n, "action") == action {
			if formNode != nil {
				s.t.Fatal("ambiguous actual form")
			}
			formNode = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if formNode == nil || attr(formNode, "method") != "post" || !strings.HasPrefix(action, "/ui/tenants/") {
		s.t.Fatal("actual mutation form missing")
	}
	values := url.Values{}
	var fields func(*html.Node)
	fields = func(n *html.Node) {
		name := attr(n, "name")
		if n.Type == html.ElementNode && n.Data == "input" && name != "" {
			values.Set(name, attr(n, "value"))
		}
		if n.Type == html.ElementNode && n.Data == "select" && name != "" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || c.Data != "option" {
					continue
				}
				v := attr(c, "value")
				if v == "" && c.FirstChild != nil {
					v = c.FirstChild.Data
				}
				if !values.Has(name) {
					values.Set(name, v)
				}
				for _, a := range c.Attr {
					if a.Key == "selected" {
						values.Set(name, v)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			fields(c)
		}
	}
	fields(formNode)
	for _, name := range []string{"csrf", "idempotency_key", "expected_desired_revision"} {
		if values.Get(name) == "" {
			s.t.Fatal("rendered form hidden contract absent")
		}
	}
	for key, value := range overrides {
		if !values.Has(key) || key == "csrf" || key == "idempotency_key" || key == "expected_desired_revision" || key == "confirm" {
			s.t.Fatal("cannot replace actual form hidden fields")
		}
		values.Set(key, value)
	}
	session := s.sessions[who]
	r, err := http.NewRequestWithContext(s.ctx, "POST", session.Config.BaseURL+action, strings.NewReader(values.Encode()))
	if err != nil {
		s.t.Fatal(err)
	}
	r.Header.Set("Origin", session.Config.BaseURL)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := session.Client.Do(r)
	if err != nil {
		s.t.Fatal("real UI submission unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 512<<10))
	if response.StatusCode != 303 {
		s.t.Fatalf("real form %s HTTP %d; request_id=%s", action, response.StatusCode, response.Header.Get("X-Request-ID"))
	}
	u, err := url.Parse(response.Header.Get("Location"))
	if err != nil || u.Host != "" || !strings.HasPrefix(u.Path, "/tenants/team-a/apps/") {
		s.t.Fatal("actual form redirect contract invalid")
	}
	app := strings.TrimPrefix(u.Path, "/tenants/team-a/apps/")
	op := u.Query().Get("operation")
	if _, err = uuid.Parse(app); err != nil {
		s.t.Fatal("application identity invalid")
	}
	if _, err = uuid.Parse(op); err != nil {
		s.t.Fatal("operation identity invalid")
	}
	var persisted domain.Operation
	s.api(who, "GET", "/api/v1/operations/"+op, "", nil, 200, &persisted)
	if persisted.AppID != app || persisted.RequestID == "" {
		s.t.Fatal("real UI intent not persisted")
	}
	return domain.Accepted{ApplicationID: app, OperationID: op}
}

func (s *suite) lesson() {
	w := s.workers[s.A]
	name := "lesson-" + uuid.NewString()[:8]
	a := s.submitForm("owner-a", "/tenants/team-a", "/ui/tenants/team-a/apps", map[string]string{"name": name, "cluster_id": s.A})
	s.wait("owner-a", "team-a", a, "SUCCEEDED", 1)
	s.quote(w, "hf-team-a", name, 18087)
	page := "/tenants/team-a/apps/" + a.ApplicationID
	action := "/ui/tenants/team-a/apps/" + a.ApplicationID
	good := s.submitForm("owner-a", page, action+"/releases", map[string]string{"image": "pricing-api:v2-good", "replicas": "2"})
	s.wait("owner-a", "team-a", good, "SUCCEEDED", 2)
	dep := &appsv1.Deployment{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: name}, dep); err != nil {
		s.t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 2 {
		s.t.Fatal("real form replicas update missing")
	}
	dep.Spec.Replicas = p(int32(1))
	if err := w.Update(s.ctx, dep); err != nil {
		s.t.Fatal(err)
	}
	eventually(s.t, s.ctx, time.Minute, "UI desired replicas drift repair", func() bool {
		if err := w.Get(s.ctx, client.ObjectKeyFromObject(dep), dep); err != nil {
			s.t.Fatal(err)
		}
		return dep.Spec.Replicas != nil && *dep.Spec.Replicas == 2 && dep.Status.ObservedGeneration == dep.Generation && dep.Status.ReadyReplicas == 2 && dep.Status.UpdatedReplicas == 2 && dep.Status.Replicas == 2
	})
	broken := s.submitForm("owner-a", page, action+"/releases", map[string]string{"image": "pricing-api:v2-broken"})
	d := s.wait("owner-a", "team-a", broken, "FAILED", 3)
	if d.Application.LastSuccessfulRevision != 2 || d.Releases[0].State != "FAILED" || len(d.Releases) != 3 {
		s.t.Fatal("broken UI deployment lost prior success/history")
	}
	var brokenOp domain.Operation
	s.api("owner-a", "GET", "/api/v1/operations/"+broken.OperationID, "", nil, 200, &brokenOp)
	if brokenOp.ErrorCode != "ProgressDeadlineExceeded" || brokenOp.RequestID == "" {
		s.t.Fatalf("real readiness failure cause missing: %+v", brokenOp)
	}
	ui := s.api("owner-a", "GET", page, "", nil, 200, nil)
	if !bytes.Contains(ui, []byte(brokenOp.ErrorCode)) || !bytes.Contains(ui, []byte(brokenOp.RequestID)) {
		s.t.Fatal("real failure absent from rendered detail")
	}
	s.quote(w, "hf-team-a", name, 18087)
	rollback := s.submitForm("owner-a", page+"?confirm=rollback&target=1", action+"/rollback", nil)
	d = s.wait("owner-a", "team-a", rollback, "SUCCEEDED", 4)
	if len(d.Releases) != 4 || d.Releases[0].RollbackSourceRevision != 1 || d.Releases[1].State != "FAILED" || d.Releases[0].Spec.Image != "pricing-api:v1" {
		s.t.Fatal("confirmed rollback rewrote failed history")
	}
	s.quote(w, "hf-team-a", name, 18087)
	deleted := s.submitForm("owner-a", page+"?confirm=delete", action+"/delete", nil)
	eventually(s.t, s.ctx, time.Minute, "confirmed UI deletion and actual GC", func() bool {
		var op domain.Operation
		s.api("owner-a", "GET", "/api/v1/operations/"+deleted.OperationID, "", nil, 200, &op)
		return op.State == "SUCCEEDED" && apierrors.IsNotFound(w.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: name}, &appsv1.Deployment{})) && apierrors.IsNotFound(w.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: name}, &corev1.Service{}))
	})
	s.t.Log("PASS actual rendered forms: deploy/HTTP → v2-good/replicas 2/drift → broken readiness/history/UI request_id → confirmed rollback/new revision/HTTP → confirmed delete/GC")
	s.recovery()
}

func (s *suite) recovery() {
	w := s.workers[s.B]
	dep := &appsv1.Deployment{}
	if err := w.Get(s.ctx, client.ObjectKey{Namespace: bootstrap.ControlNamespace, Name: "hugofleet-agent"}, dep); err != nil {
		s.t.Fatal(err)
	}
	manager := bootstrap.Manager
	if s.stage == "m5" {
		manager = "Helm"
	}
	if dep.Labels[bootstrap.ManagedLabel] != manager || dep.Labels[bootstrap.ClusterLabel] != s.B {
		s.t.Fatal("foreign Agent cannot be scaled")
	}
	if s.stage == "m5" && (dep.Annotations["meta.helm.sh/release-name"] != "hugofleet-worker" || dep.Annotations["meta.helm.sh/release-namespace"] != bootstrap.ControlNamespace) {
		s.t.Fatal("Agent is outside the exact M5 Helm release")
	}
	dep.Spec.Replicas = p(int32(0))
	if err := w.Update(s.ctx, dep); err != nil {
		s.t.Fatal(err)
	}
	restored := false
	defer func() {
		if !restored {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := w.Get(ctx, client.ObjectKeyFromObject(dep), dep); err == nil {
				dep.Spec.Replicas = p(int32(1))
				if err = w.Update(ctx, dep); err != nil {
					s.t.Errorf("restore owned Agent: %v", err)
				}
			} else {
				s.t.Errorf("read owned Agent for restore: %v", err)
			}
		}
	}()
	registry := func(connected bool) bool {
		var items []domain.Cluster
		s.api("admin", "GET", "/api/v1/clusters", "", nil, 200, &items)
		for _, c := range items {
			if c.ID == s.B {
				return c.LastSeenAt > 0 && ((connected && c.State == "CONNECTED") || (!connected && c.State == "DISCONNECTED" && c.LastError != ""))
			}
		}
		return false
	}
	eventually(s.t, s.ctx, time.Minute, "real B disconnect with retained observation time", func() bool { return registry(false) })
	s.quote(w, "hf-team-b", "pricing-b", 18086)
	if err := w.Get(s.ctx, client.ObjectKeyFromObject(dep), dep); err != nil {
		s.t.Fatal(err)
	}
	dep.Spec.Replicas = p(int32(1))
	if err := w.Update(s.ctx, dep); err != nil {
		s.t.Fatal(err)
	}
	restored = true
	eventually(s.t, s.ctx, 2*time.Minute, "real B reconnect", func() bool { return registry(true) })
	prefix := "HF_" + strings.ToUpper(s.stage)
	hub := os.Getenv(prefix + "_HUB")
	record, err := os.ReadFile(filepath.Join(os.Getenv(prefix+"_STATE"), "hub.id"))
	if err != nil {
		s.t.Fatal(err)
	}
	identity, err := exec.CommandContext(s.ctx, "docker", "inspect", "--format", "{{.Id}}", hub).Output()
	if err != nil || strings.TrimSpace(string(record)) != strings.TrimSpace(string(identity)) {
		s.t.Fatal("Hub ownership mismatch")
	}
	run := func(args ...string) {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "docker", args...).Run(); err != nil {
			s.t.Fatalf("owned Hub lifecycle %v: %v", args, err)
		}
	}
	run("stop", "--time", "15", hub)
	started := false
	defer func() {
		if !started {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := exec.CommandContext(ctx, "docker", "start", hub).Run(); err != nil {
				s.t.Errorf("restore owned Hub: %v", err)
			}
		}
	}()
	aw := s.workers[s.A]
	appDep := &appsv1.Deployment{}
	if err = aw.Get(s.ctx, client.ObjectKey{Namespace: "hf-team-a", Name: "pricing-api"}, appDep); err != nil {
		s.t.Fatal(err)
	}
	appDep.Spec.Replicas = p(int32(0))
	if err = aw.Update(s.ctx, appDep); err != nil {
		s.t.Fatal(err)
	}
	eventually(s.t, s.ctx, time.Minute, "Operator repairs drift while Hub stopped", func() bool {
		if err := aw.Get(s.ctx, client.ObjectKeyFromObject(appDep), appDep); err != nil {
			s.t.Fatal(err)
		}
		return appDep.Spec.Replicas != nil && *appDep.Spec.Replicas == 1 && appDep.Status.ReadyReplicas == 1 && appDep.Status.UpdatedReplicas == 1 && appDep.Status.Replicas == 1 && appDep.Status.ObservedGeneration == appDep.Generation
	})
	s.quote(aw, "hf-team-a", "pricing-api", 18085)
	run("start", hub)
	started = true
	eventually(s.t, s.ctx, time.Minute, "Hub actual public health after restart", func() bool {
		request, err := http.NewRequestWithContext(s.ctx, "GET", s.sessions["admin"].Config.BaseURL+"/healthz", nil)
		if err != nil {
			s.t.Fatal(err)
		}
		r, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if err != nil {
			return false
		}
		defer r.Body.Close()
		return r.StatusCode == 200
	})
	eventually(s.t, s.ctx, 2*time.Minute, "real channels restored after persistent Hub restart", func() bool {
		var items []domain.Cluster
		s.api("admin", "GET", "/api/v1/clusters", "", nil, 200, &items)
		return len(items) == 2 && items[0].State == "CONNECTED" && items[1].State == "CONNECTED"
	})
	var apps domain.Page[domain.Application]
	s.api("developer-a", "GET", "/api/v1/tenants/team-a/apps", "", nil, 200, &apps)
	if len(apps.Items) < 1 {
		s.t.Fatal("Hub restart lost scoped application history/session")
	}
	s.t.Log(fmt.Sprintf("PASS actual B offline/reconnect/service continuity; Hub %s stopped/started with original DB/session and autonomous Worker drift repair", hub))
}
