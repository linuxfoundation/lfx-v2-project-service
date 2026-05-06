// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

var _ = Service("project-service", func() {
	Method("create-project-link", func() {
		Description("Create a new link for a project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceNameAttribute("name", "Link display name")
			Attribute("url", String, "The URL of the link", func() {
				Format(FormatURI)
				Example("https://example.com")
			})
			ResourceDescriptionAttribute("description", "A description of the link")
			ResourceUIDAttribute("folder_uid", "Folder UID to place the link in (optional)")
			Required("uid", "name", "url")
		})

		Result(ProjectLink)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Project not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects/{uid}/links")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("x_sync:X-Sync")
			Response(StatusCreated)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-project-link", func() {
		Description("Get a single project link.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("link_uid", "Link UID")
			Required("uid", "link_uid")
		})

		Result(func() {
			Attribute("link", ProjectLink)
			EtagAttribute()
			Required("link")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/links/{link_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("link_uid")
			})
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Body("link")
				Header("etag:ETag")
			})
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("list-project-links", func() {
		Description("List all links for a project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			Required("uid")
		})

		Result(func() {
			Attribute("links", ArrayOf(ProjectLink), "List of project links")
			Required("links")
		})

		Error("NotFound", NotFoundError, "Project not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/links")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Response(StatusOK)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("delete-project-link", func() {
		Description("Delete a project link.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			IfMatchAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("link_uid", "Link UID")
			Required("uid", "link_uid")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("Conflict", ConflictError, "Revision mismatch")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{uid}/links/{link_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("link_uid")
			})
			Header("bearer_token:Authorization")
			Header("x_sync:X-Sync")
			Header("if_match:If-Match")
			Response(StatusNoContent)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})
})
