package httpserver

import (
	accessmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/access"
	accesshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/access/handler"
	activitymodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity"
	activityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/handler"
	logsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs"
	logshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/handler"
	searchmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search"
	searchhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
)

// moduleHandlers carries the handlers of self-registering modules.
type moduleHandlers struct {
	access   *accesshandler.Handler
	search   *searchhandler.Handler
	activity *activityhandler.Handler
	logs     *logshandler.Handler
}

// selfRegisteredModules is the single list of modules mounted through
// module.Router (docs/design/module-registry.md). server.go mounts it with
// real handlers; modules_test.go mounts it record-only to check OpenAPI.
func selfRegisteredModules(h moduleHandlers) []module.Module {
	return []module.Module{
		accessmodule.Module{H: h.access},
		searchmodule.Module{H: h.search},
		activitymodule.Module{H: h.activity},
		logsmodule.Module{H: h.logs},
	}
}
