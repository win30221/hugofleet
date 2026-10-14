package repository

import (
	"context"
	"errors"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
)

type Overview struct {
	Clusters     int
	Connected    int
	Applications int
	Ready        int
	Degraded     int
	Unknown      int
	Recent       []RecentOperation
}
type RecentOperation struct {
	domain.Operation
	TenantID, ClusterID, Name string
}

// Counts use the full authorized scope, rather than a truncated UI list.
func (s *Store) Overview(ctx context.Context) (v Overview, err error) {
	u, err := s.CurrentUser(ctx)
	if err != nil {
		return v, err
	}
	clusters, err := s.VisibleClusters(ctx)
	if err != nil {
		return v, err
	}
	v.Clusters = len(clusters)
	for _, c := range clusters {
		if c.State == "CONNECTED" {
			v.Connected++
		}
	}
	err = s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(status='READY'),0),COALESCE(sum(status='DEGRADED'),0),COALESCE(sum(status='UNKNOWN'),0) FROM applications a WHERE a.status!='DELETED' AND (? OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=a.tenant_id AND m.user_id=?))`, u.PlatformAdmin, u.ID).Scan(&v.Applications, &v.Ready, &v.Degraded, &v.Unknown)
	if err != nil {
		return v, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.id,o.app_id,COALESCE(o.release_id,''),o.command_id,o.type,o.state,o.error_code,o.error_message,o.created_at,o.deadline_at,o.finished_at,o.request_id,o.traceparent,o.tracestate,a.tenant_id,a.cluster_id,a.name FROM operations o JOIN applications a ON a.id=o.app_id WHERE ? OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=a.tenant_id AND m.user_id=?) ORDER BY o.created_at DESC,o.rowid DESC LIMIT 20`, u.PlatformAdmin, u.ID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	v.Recent = []RecentOperation{}
	for rows.Next() {
		var x RecentOperation
		err = rows.Scan(&x.ID, &x.AppID, &x.ReleaseID, &x.CommandID, &x.Type, &x.State, &x.ErrorCode, &x.ErrorMessage, &x.CreatedAt, &x.DeadlineAt, &x.FinishedAt, &x.RequestID, &x.Traceparent, &x.Tracestate, &x.TenantID, &x.ClusterID, &x.Name)
		if err != nil {
			return v, err
		}
		v.Recent = append(v.Recent, x)
	}
	return v, rows.Err()
}

func (s *Store) DashboardApps(ctx context.Context, tenant, cluster, after string, limit int) ([]domain.Application, error) {
	u, err := s.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, domain.Problem("INVALID_ARGUMENT", "apps limit must be 1..100")
	}
	if tenant != "" {
		if _, err = s.Authorize(ctx, tenant, "Viewer"); err != nil {
			return nil, err
		}
	}
	if cluster != "" {
		visible, e := s.VisibleClusters(ctx)
		if e != nil {
			return nil, e
		}
		allowed := false
		for _, c := range visible {
			if c.ID == cluster {
				allowed = true
			}
		}
		if !allowed {
			return nil, domain.Problem("PERMISSION_DENIED", "cluster is outside current visibility")
		}
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+appColumns+` FROM applications WHERE (?='' OR tenant_id=?) AND (?='' OR cluster_id=?) AND id>? AND (? OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=applications.tenant_id AND m.user_id=?)) ORDER BY id LIMIT ?`, tenant, tenant, cluster, cluster, after, u.PlatformAdmin, u.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Application{}
	for rows.Next() {
		a, e := appScan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, errors.Join(rows.Err(), rows.Close())
}
