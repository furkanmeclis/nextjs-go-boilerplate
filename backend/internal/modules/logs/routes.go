package logs

import (
	logshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Module mounts the platform logs and log-retention-rule routes.
type Module struct{ H *logshandler.Handler }

func (Module) Name() string { return "logs" }

func (m Module) Routes(r *module.Router) {
	read, write := rbac.PermPlatformLogsRead, rbac.PermPlatformLogsWrite
	h := m.H
	r.Platform("GET /v1/platform/logs/meta", read, h.Meta)
	r.Platform("GET /v1/platform/logs/stats", read, h.Stats)
	r.Platform("GET /v1/platform/logs/sources", read, h.Sources)
	r.Platform("GET /v1/platform/logs", read, h.List)
	r.Platform("POST /v1/platform/logs/purge", write, h.Purge)
	r.Platform("GET /v1/platform/logs/{uuid}", read, h.Get)
	r.Platform("DELETE /v1/platform/logs/{uuid}", write, h.Delete)

	r.Platform("GET /v1/platform/log-rules/meta", read, h.RulesMeta)
	r.Platform("GET /v1/platform/log-rules", read, h.ListRules)
	r.Platform("POST /v1/platform/log-rules", write, h.CreateRule)
	r.Platform("GET /v1/platform/log-rules/{uuid}", read, h.GetRule)
	r.Platform("PATCH /v1/platform/log-rules/{uuid}", write, h.PatchRule)
	r.Platform("DELETE /v1/platform/log-rules/{uuid}", write, h.DeleteRule)
	r.Platform("POST /v1/platform/log-rules/{uuid}/run", write, h.RunRule)
}
