// The lfx-v2-project-service service.
package main

import (
	"context"
	"errors"

	projsvc "lfx-v2-project-service/gen/project_service"

	"goa.design/goa/v3/security"
)

// ProjectsService implements the projsvc.Service interface
type ProjectsService struct{}

type contextID int

const (
	nonceSize                    = 24
	anonymousPrincipal           = `_anonymous`
	accessCheckSubject           = "dev.lfx.access_check.request"
	principalContextID contextID = iota
)

// TransactionBodyStub is used to decode the OpenSearch response's "source".
// **Ensure the fields here align to the relevant `SourceIncludes`
// parameters**.
type TransactionBodyStub struct {
	ObjectRef            string      `json:"object_ref"`
	ObjectType           string      `json:"object_type"`
	ObjectID             string      `json:"object_id"`
	Public               bool        `json:"public"`
	AccessCheckObject    string      `json:"access_check_object"`
	AccessCheckRelation  string      `json:"access_check_relation"`
	HistoryCheckObject   string      `json:"history_check_object"`
	HistoryCheckRelation string      `json:"history_check_relation"`
	Data                 interface{} `json:"data"`
}

// GetProjects fetches all projects
func (s *ProjectsService) GetProjects(ctx context.Context, payload *projsvc.GetProjectsPayload) (*projsvc.GetProjectsResult, error) {

	reqLogger := logger.With("method", "GetProjects")
	reqLogger.With("request", payload).DebugContext(ctx, "request")

	if payload.PageToken != nil {
		return nil, projsvc.MakeBadRequest(errors.New("page token is not supported for projects query yet"))
	}

	// return hardcoded list for now. Implement NATS later
	id := "123"
	slug := "project-slug"
	description := "project foo is a project about bar"
	name := "Foo Foundation"
	projects := []*projsvc.Project{
		&projsvc.Project{
			ID:          &id,
			Slug:        &slug,
			Description: &description,
			Name:        &name,
			Managers:    []string{"user123", "user456"},
		},
	}

	reqLogger.Debug("returning projects", "projects", projects)

	return &projsvc.GetProjectsResult{
		Projects:     projects,
		PageToken:    nil,
		CacheControl: nil,
	}, nil

}

// Readyz checks if the service is able to take inbound requests.
func (s *ProjectsService) Readyz(ctx context.Context) ([]byte, error) {
	return []byte("OK\n"), nil
}

// Livez checks if the service is alive.
func (s *ProjectsService) Livez(context.Context) ([]byte, error) {
	// This always returns as long as the service is still running. As this
	// endpoint is expected to be used as a Kubernetes liveness check, this
	// service must likewise self-detect non-recoverable errors and
	// self-terminate.
	return []byte("OK\n"), nil
}

// JWTAuth implements Auther interface for the JWT security scheme.
func (s *ProjectsService) JWTAuth(ctx context.Context, bearerToken string, schema *security.JWTScheme) (context.Context, error) {
	// Parse the Heimdall-authorized principal from the token.
	principal, err := parsePrincipal(ctx, bearerToken)
	if err != nil {
		return ctx, err
	}
	// Return a new context containing the principal as a value.
	return context.WithValue(ctx, principalContextID, principal), nil
}
