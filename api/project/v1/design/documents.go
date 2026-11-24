// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

// UploadProjectDocumentPayload is the multipart payload for document upload.
var UploadProjectDocumentPayload = Type("UploadProjectDocumentPayload", func() {
	Description("Multipart/form-data payload for uploading a project document.")
	ResourceNameAttribute("name", "Document display name")
	ResourceDescriptionAttribute("description", "A description of the document")
	ResourceUIDAttribute("folder_uid", "Folder UID to place the document in (optional)")
	Attribute("file", Bytes, "File contents", func() {
		Example([]byte("..."))
	})
	Attribute("file_name", String, "Original file name including extension", func() {
		Example("report.pdf")
	})
	Attribute("content_type", String, "MIME type of the file", func() {
		Example("application/pdf")
	})
	Required("name", "file", "file_name", "content_type")
})

var _ = Service("project-service", func() {
	Method("upload-project-document", func() {
		Description("Upload a new document for a project (multipart/form-data).")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			Extend(UploadProjectDocumentPayload)
			Required("uid")
		})

		Result(ProjectDocument)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Project not found")
		Error("Conflict", ConflictError, "Document name already exists")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects/{uid}/documents")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Header("x_sync:X-Sync")
			MultipartRequest()
			Response(StatusCreated)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("get-project-document", func() {
		Description("Get project document metadata.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("document_uid", "Document UID")
			Required("uid", "document_uid")
		})

		Result(func() {
			Attribute("document", ProjectDocument)
			EtagAttribute()
			Required("document")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/documents/{document_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("document_uid")
			})
			Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Body("document")
				Header("etag:ETag")
			})
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("download-project-document", func() {
		Description("Download the binary file of a project document.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("document_uid", "Document UID")
			Required("uid", "document_uid")
		})

		Error("NotFound", NotFoundError, "Resource not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			GET("/projects/{uid}/documents/{document_uid}/download")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("document_uid")
			})
			Header("bearer_token:Authorization")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("update-project-document", func() {
		Description("Update a project document's name, description and folder. Full replace: omitting description clears it, and omitting folder_uid moves the document to the project root. The uploaded file is not changed.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			IfMatchAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("document_uid", "Document UID")
			Attribute("name", String, "Document display name; unique within the project", func() {
				MinLength(1)
				MaxLength(255)
				Example("My Resource")
			})
			Attribute("description", String, "A description of the document", func() {
				MaxLength(4096)
				Example("A description of the resource")
			})
			ResourceUIDAttribute("folder_uid", "Folder UID to place the document in (omit for the project root)")
			Required("uid", "document_uid", "name")
		})

		Result(ProjectDocument)

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Document or folder not found")
		Error("Conflict", ConflictError, "Document name already exists, or the revision is stale")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			PUT("/projects/{uid}/documents/{document_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("document_uid")
			})
			Header("bearer_token:Authorization")
			Header("x_sync:X-Sync")
			Header("if_match:If-Match")
			Response(StatusOK)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("Conflict", StatusConflict)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("delete-project-document", func() {
		Description("Delete a project document.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			XSyncAttribute()
			IfMatchAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			ResourceUIDAttribute("document_uid", "Document UID")
			Required("uid", "document_uid")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Resource not found")
		Error("Conflict", ConflictError, "Revision mismatch")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{uid}/documents/{document_uid}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("document_uid")
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
