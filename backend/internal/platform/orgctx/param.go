package orgctx

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// Param returns the organization id of the request scope as a nullable SQL
// parameter: set inside /v1/tenant/* requests (and org-scoped jobs), NULL on
// platform/CMS paths. Engines stamp it on the rows they create
// (activity_events, notifications, export/import/bulk jobs).
func Param(ctx context.Context) pgtype.Int8 {
	if s, ok := From(ctx); ok && s.ID > 0 {
		return pgtype.Int8{Int64: s.ID, Valid: true}
	}
	return pgtype.Int8{}
}

// WithOrganizationID scopes a background context (e.g. a worker processing
// an org-stamped job) to an organization by id, so adapters reading
// From/Param see the same organization as the request that queued the job.
func WithOrganizationID(ctx context.Context, id pgtype.Int8) context.Context {
	if !id.Valid || id.Int64 <= 0 {
		return ctx
	}
	return WithScope(ctx, Scope{ID: id.Int64})
}
