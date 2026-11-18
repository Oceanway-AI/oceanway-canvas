package app

import "yingce/backend/internal/repository"

// Creative domain tests exercise retained source independently of the HTTP
// composition root. Executables always construct NewStudio with a Gateway.
func New(repo *repository.Repository, dataDir string) *Service {
	service := newService(repo, dataDir, nil)
	service.pluginRuntime, service.pluginRuntimeErr = newPluginRuntime(dataDir)
	service.taskTerminalCoordinator = newTaskTerminalCoordinator(service)
	service.taskRouteExecutor = newTaskRouteExecutor(service)
	return service
}
