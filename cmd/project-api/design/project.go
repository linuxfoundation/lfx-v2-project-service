// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package design

import (
	. "goa.design/goa/v3/dsl"
)

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
			Token("bearer_token", String, func() {
				Description("JWT token issued by Heimdall")
				Example("eyJhbGci...")
			})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
			Attribute("page_size", Int, "Page size", func() {
				// TODO: update these validations to reasonable values once API performance efficiency is accounted for.
				Default(10)
				Minimum(1)
				Maximum(100)
				Example(10)
			})
			Attribute("page_token", String, "opaque page token", func() {
				Description("Token to get the next page of results, if available")
				Example("****")
			})
		})

		Result(func() {
			Attribute("projects", ArrayOf(Project), "Resources found", func() {})
			Attribute("page_token", String, "Opaque token if more results are available", func() {
				Example("****")
			})
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
			Param("page_token")
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
			Token("bearer_token", String, func() {
				Description("JWT token issued by Heimdall")
				Example("eyJhbGci...")
			})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
			Attribute("slug", String, "Project slug, a short slugified name of the project", func() {
				Example("project-slug")
			})
			Attribute("description", String, "A description of the project", func() {
				Example("project foo is a project about bar")
			})
			Attribute("name", String, "The pretty name of the project", func() {
				Example("Foo Foundation")
			})
			Attribute("managers", ArrayOf(String), "A list of project managers", func() {
				Example([]string{"user123", "user456"})
			})
			Required("slug", "name", "managers")
		})

		Result(Project)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("Conflict", ConflictError, "Conflict")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects")
			Param("version:v")
			Header("bearer_token:Authorization")
			Response(StatusOK)
			Response("BadRequest", StatusBadRequest)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-one-project", func() {
		Description("Get a single project.")

		Security(JWTAuth)

		Payload(func() {
			Token("bearer_token", String, func() {
				Description("JWT token issued by Heimdall")
				Example("eyJhbGci...")
			})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
			Attribute("project_id", String, "Project ID", func() {
				Example("123")
			})
		})

		Result(func() {
			Attribute("project", Project)
			Attribute("etag", String, "ETag header value")
			Required("project")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{project_id}")
			Param("version:v")
			Param("project_id")
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

	Method("update-project", func() {
		Description("Update an existing project.")

		Security(JWTAuth)

		Payload(func() {
			Token("bearer_token", String, func() {
				Description("JWT token issued by Heimdall")
				Example("eyJhbGci...")
			})
			Attribute("etag", String, "ETag header value", func() {
				Example("123")
			})
			Attribute("project_id", String, "Project ID", func() {
				Example("123")
			})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
			Attribute("slug", String, "Project slug, a short slugified name of the project", func() {
				Example("project-slug")
			})
			Attribute("description", String, "A description of the project", func() {
				Example("project foo is a project about bar")
			})
			Attribute("name", String, "The pretty name of the project", func() {
				Example("Foo Foundation")
			})
			Attribute("managers", ArrayOf(String), "A list of project managers", func() {
				Example([]string{"user123", "user456"})
			})
			Required("slug", "name", "managers")
		})

		Result(Project)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			PUT("/projects/{project_id}")
			Params(func() {
				Param("version:v")
				Param("project_id")
			})
			Body(func() {
				Attribute("slug")
				Attribute("description")
				Attribute("name")
				Attribute("managers")
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
			Token("bearer_token", String, func() {
				Description("JWT token issued by Heimdall")
				Example("eyJhbGci...")
			})
			Attribute("etag", String, "ETag header value", func() {
				Example("123")
			})
			Attribute("project_id", String, "Project ID", func() {
				Example("123")
			})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("BadRequest", BadRequestError, "Bad request")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{project_id}")
			Params(func() {
				Param("version:v")
				Param("project_id")
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
