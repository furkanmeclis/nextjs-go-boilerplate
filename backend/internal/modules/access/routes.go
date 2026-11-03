package access

import (
	accesshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/access/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Module mounts the access policy routes.
type Module struct{ H *accesshandler.Handler }

func (Module) Name() string { return "access" }

func (m Module) Routes(r *module.Router) {
	r.Platform("GET /v1/platform/access/settings", rbac.PermPlatformAccessRead, m.H.GetSettings)
	r.Platform("PATCH /v1/platform/access/settings", rbac.PermPlatformAccessWrite, m.H.PatchSettings)
}
