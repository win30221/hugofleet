package repository

import (
	"context"

	"github.com/win30221/hugofleet/stages/m2/hub/domain"
)

func validatePageLimit(limit int) error {
	if limit < 1 || limit > 100 {
		return domain.Problem("INVALID_ARGUMENT", "page limit must be 1..100")
	}
	return nil
}

// Callers fetch one extra row so a full final page has no spurious next cursor.
func boundedPage[T any](items []T, limit int, id func(T) string) domain.Page[T] {
	p := domain.Page[T]{Items: items}
	if len(items) > limit {
		p.NextAfter = id(items[limit-1])
		p.Items = items[:limit]
	}
	return p
}

func (s *Store) ScopedAppsPage(ctx context.Context, tenant, after string, limit int) (domain.Page[domain.Application], error) {
	if _, err := s.Authorize(ctx, tenant, "Viewer"); err != nil {
		return domain.Page[domain.Application]{}, err
	}
	if err := validatePageLimit(limit); err != nil {
		return domain.Page[domain.Application]{}, err
	}
	items, err := s.Apps(ctx, tenant, after, limit+1)
	if err != nil {
		return domain.Page[domain.Application]{}, err
	}
	return boundedPage(items, limit, func(a domain.Application) string { return a.ID }), nil
}
