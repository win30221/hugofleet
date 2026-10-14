package repository

import (
	"context"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"time"
)

// ConfigureRemote refuses to mix learning modes or silently migrate old apps.
func (s *Store) ConfigureRemote(ctx context.Context, ids []string) error {
	if s.secure {
		return fmt.Errorf("M3 cannot open an OIDC database; use the M4 Hub")
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		return fmt.Errorf("remote registry requires two distinct cluster IDs")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM applications WHERE cluster_id=?", domain.ClusterID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("M3 requires its own database; this database contains M2 applications")
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM clusters WHERE id NOT IN (?,?,?)", domain.ClusterID, ids[0], ids[1]).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("database belongs to a different remote cluster registry")
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM clusters WHERE id=?", domain.ClusterID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO clusters(id,name,state,last_seen_at,registered_at) VALUES(?,?,'DISCONNECTED',0,?)", id, id, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) CheckLocal(ctx context.Context) error {
	if s.secure {
		return fmt.Errorf("M2 cannot open an OIDC database; use the M4 Hub")
	}
	var count int
	if e := s.db.QueryRowContext(ctx, "SELECT count(*) FROM clusters WHERE id!=?", domain.ClusterID).Scan(&count); e != nil {
		return e
	}
	if count != 0 {
		return fmt.Errorf("M2 requires its own database; use M3 mode for this registry")
	}
	return nil
}
func (s *Store) ClusterConnectivity(ctx context.Context, id string, connected bool, seen int64, version string) error {
	state := "DISCONNECTED"
	message := "No active authenticated Agent session; check endpoint, credentials and Worker logs"
	if connected {
		state = "CONNECTED"
		message = ""
	}
	_, err := s.db.ExecContext(ctx, "UPDATE clusters SET state=?,last_seen_at=MAX(last_seen_at,?),agent_version=?,last_error=? WHERE id=?", state, seen, version, message, id)
	return err
}
