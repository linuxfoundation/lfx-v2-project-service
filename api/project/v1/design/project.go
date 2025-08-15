// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package design contains the DSL for the project service Goa API generation.
package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

// JWTAuth is the DSL JWT security type for authentication.
var JWTAuth = JWTSecurity("jwt", func() {
	Description("Heimdall authorization")
})

var _ = Service("project-service", func() {
	Description("The project service provides LFX Project resources.")

	// TODO: delete this endpoint once the query service is implemented
	Method("get-projects", func() {
		Description("Get all projects.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
		})

		Result(func() {
			Attribute("projects", ArrayOf(ProjectFull), "Resources found", func() {})
			Attribute("cache_control", String, "Cache control header", func() {
				Example("public, max-age=300")
			})
			Required("projects")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects")
			Param("version:v")
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Header("cache_control:Cache-Control")
			})
			Response("BadRequest", StatusBadRequest)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("create-project", func() {
		Description("Create a new project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectSlugAttribute()
			ProjectDescriptionAttribute()
			ProjectNameAttribute()
			ProjectPublicAttribute()
			ProjectParentUIDAttribute()
			ProjectStageAttribute()
			ProjectCategoryAttribute()
			ProjectFundingModelAttribute()
			ProjectCharterURLAttribute()
			ProjectLegalEntityTypeAttribute()
			ProjectLegalEntityNameAttribute()
			ProjectLegalParentUIDAttribute()
			ProjectEntityDissolutionDateAttribute()
			ProjectEntityFormationDocumentURLAttribute()
			ProjectAutojoinEnabledAttribute()
			ProjectFormationDateAttribute()
			ProjectLogoURLAttribute()
			ProjectRepositoryURLAttribute()
			ProjectWebsiteURLAttribute()
			ProjectAnnouncementDateAttribute()
			ProjectMissionStatementAttribute()
			ProjectWritersAttribute()
			ProjectMeetingCoordinatorsAttribute()
			ProjectAuditorsAttribute()

			// TODO: figure out what the required attributes are for projects
			// Same requirements apply to PUT endpoints.
			Required("slug", "description", "name", "parent_uid")
		})

		Result(ProjectFull)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("Conflict", ConflictError, "Conflict")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects")
			Param("version:v")
			Header("bearer_token:Authorization")
			Response(StatusCreated)
			Response("BadRequest", StatusBadRequest)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-one-project-base", func() {
		Description("Get a single project's base information.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
		})

		Result(func() {
			Attribute("project", ProjectBase)
			EtagAttribute()
			Required("project")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}")
			Param("version:v")
			Param("uid")
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Body("project")
				Header("etag:ETag")
			})
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-one-project-settings", func() {
		Description("Get a single project's settings.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
		})

		Result(func() {
			Attribute("project_settings", ProjectSettings)
			EtagAttribute()
			Required("project_settings")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/settings")
			Param("version:v")
			Param("uid")
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Body("project_settings")
				Header("etag:ETag")
			})
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("update-project-base", func() {
		Description("Update an existing project's base information.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			EtagAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ProjectSlugAttribute()
			ProjectDescriptionAttribute()
			ProjectNameAttribute()
			ProjectPublicAttribute()
			ProjectParentUIDAttribute()
			ProjectStageAttribute()
			ProjectCategoryAttribute()
			ProjectFundingModelAttribute()
			ProjectCharterURLAttribute()
			ProjectLegalEntityTypeAttribute()
			ProjectLegalEntityNameAttribute()
			ProjectLegalParentUIDAttribute()
			ProjectEntityDissolutionDateAttribute()
			ProjectEntityFormationDocumentURLAttribute()
			ProjectAutojoinEnabledAttribute()
			ProjectFormationDateAttribute()
			ProjectLogoURLAttribute()
			ProjectRepositoryURLAttribute()
			ProjectWebsiteURLAttribute()
			Required("slug", "description", "name", "parent_uid")
		})

		Result(ProjectBase)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("Conflict", ConflictError, "Conflict")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			PUT("/projects/{uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("etag:ETag")
			Response(StatusOK)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("update-project-settings", func() {
		Description("Update an existing project's settings.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			EtagAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ProjectMissionStatementAttribute()
			ProjectAnnouncementDateAttribute()
			ProjectWritersAttribute()
			ProjectMeetingCoordinatorsAttribute()
			ProjectAuditorsAttribute()
		})

		Result(ProjectSettings)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			PUT("/projects/{uid}/settings")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("etag:ETag")
			Response(StatusOK)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("delete-project", func() {
		Description("Delete an existing project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			EtagAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("BadRequest", BadRequestError, "Bad request")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("etag:ETag")
			Response(StatusNoContent)
			Response("NotFound", StatusNotFound)
			Response("BadRequest", StatusBadRequest)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("readyz", func() {
		Description("Check if the service is able to take inbound requests.")
		Result(Bytes, func() {
			Example("OK")
		})
		Error("ServiceUnavailable", ServiceUnavailableError, "Service is unavailable")
		HTTP(func() {
			GET("/readyz")
			Response(StatusOK, func() {
				ContentType("text/plain")
			})
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("livez", func() {
		Description("Check if the service is alive.")
		Result(Bytes, func() {
			Example("OK")
		})
		HTTP(func() {
			GET("/livez")
			Response(StatusOK, func() {
				ContentType("text/plain")
			})
		})
	})

	// Serve the file gen/http/openapi3.json for requests sent to /openapi.json.
	Files("/openapi.json", "gen/http/openapi3.json")
})
