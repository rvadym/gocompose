// Package goroute generates a GetRoutes function returning infra.HandlersConfigs for a godi container.
//
// It scans Go packages for //router:<method> <path> directives (e.g. //router:get /index) attached to handler
// constructors that are also tagged //di:wire, and writes router.go next to
// the generated container. Each route maps its method and path to the Container field
// godi creates for the constructor's package (api_index -> ApiIndex).
package goroute
