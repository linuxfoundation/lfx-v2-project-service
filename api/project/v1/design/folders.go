// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

var _ = Service("project-service", func() {
	Method("create-project-folder", func() {
		Description("Create a new folder for a project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceNameAttribute("name", "Folder display name")
			Required("uid", "name")
		})

		Result(ProjectFolder)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Project not found")
		Error("Conflict", ConflictError, "Folder name already exists")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects/{uid}/folders")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("x_sync:X-Sync")
			Response(StatusCreated)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-project-folder", func() {
		Description("Get a single project folder.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("folder_uid", "Folder UID")
			Required("uid", "folder_uid")
		})

		Result(func() {
			Attribute("folder", ProjectFolder)
			EtagAttribute()
			Required("folder")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/folders/{folder_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("folder_uid")
			})
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Body("folder")
				Header("etag:ETag")
			})
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("delete-project-folder", func() {
		Description("Delete a project folder. The folder must be empty.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			IfMatchAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("folder_uid", "Folder UID")
			Required("uid", "folder_uid")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("Conflict", ConflictError, "Folder not empty or revision mismatch")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{uid}/folders/{folder_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("folder_uid")
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
