package activity

import (
	activityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Module mounts the platform activity (audit log) routes.
type Module struct{ H *activityhandler.Handler }

func (Module) Name() string { return "activity" }

func (m Module) Routes(r *module.Router) {
	read := rbac.PermPlatformActivityRead
	r.Platform("GET /v1/platform/activity/meta", read, m.H.Meta)
	r.Platform("GET /v1/platform/activity", read, m.H.List)
}
