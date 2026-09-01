// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

// MarketingOpsUsernameAttribute is the DSL attribute for a marketing ops member's username.
func MarketingOpsUsernameAttribute() {
	Attribute("username", String, "The username/LFID of the user to grant or revoke marketing ops access", func() {
		Example("johndoe123")
	})
}

var _ = Service("project-service", func() {
	Method("add-project-marketing-ops-member", func() {
		Description("Grant a user Marketing Ops access (Campaign Impact and Campaigns) scoped to a single project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MarketingOpsUsernameAttribute()
			Required("uid", "username")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Project not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			POST("/projects/{uid}/marketing-ops-members")
			Params(func() {
				Param("version:v")
				Param("uid")
			})
			Header("bearer_token:Authorization")
			Response(StatusCreated)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})

	Method("remove-project-marketing-ops-member", func() {
		Description("Revoke a user's Marketing Ops access for a single project.")

		Security(JWTAuth)

		Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MarketingOpsUsernameAttribute()
			Required("uid", "username")
		})

		Error("BadRequest", BadRequestError, "Bad request")
		Error("NotFound", NotFoundError, "Project not found")
		Error("InternalServerError", InternalServerError, "Internal server error")
		Error("ServiceUnavailable", ServiceUnavailableError, "Service unavailable")

		HTTP(func() {
			DELETE("/projects/{uid}/marketing-ops-members/{username}")
			Params(func() {
				Param("version:v")
				Param("uid")
				Param("username")
			})
			Header("bearer_token:Authorization")
			Response(StatusNoContent)
			Response("BadRequest", StatusBadRequest)
			Response("NotFound", StatusNotFound)
			Response("InternalServerError", StatusInternalServerError)
			Response("ServiceUnavailable", StatusServiceUnavailable)
		})
	})
})
