// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	"goa.design/goa/v3/dsl"
)

var _ = dsl.API("membership", func() {
	dsl.Title("Membership Management Service")
})

// JWTAuth is the DSL JWT security type for authentication.
var JWTAuth = dsl.JWTSecurity("jwt", func() {
	dsl.Description("Heimdall authorization")
})

// Service describes the membership service
var _ = dsl.Service("membership-service", func() {
	dsl.Description("Membership management service — project-scoped drill-down API for tiers, memberships, and key contacts")

	// ── Tiers (Product2 per project) ────────────────────────────────────────

	dsl.Method("list-project-tiers", func() {
		dsl.Description("List membership tiers (Product2 records) for a specific project")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("tiers", dsl.ArrayOf(MembershipTierResponse), "List of membership tiers")
			dsl.Required("tiers")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Project not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/tiers")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK)
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("get-project-tier", func() {
		dsl.Description("Get a specific membership tier by UID")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			TierUIDAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("tier", MembershipTierResponse, "Membership tier details")
			dsl.Required("tier")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Tier not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/tiers/{tier_uid}")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("tier_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK, func() {
				dsl.Body("tier")
			})
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	// ── Memberships (Asset per project) ─────────────────────────────────────

	dsl.Method("list-project-memberships", func() {
		dsl.Description("List memberships (Asset records) for a specific project, with denormalized company attributes")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			PageSizeAttribute()
			PageTokenAttribute()
			SortAttribute()
			FilterAttribute()
			SearchNameAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("memberships", dsl.ArrayOf(ProjectMembershipResponse), "List of project memberships")
			dsl.Attribute("metadata", ListMetadata, "Pagination metadata")
			dsl.Required("memberships", "metadata")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Project not found")
		dsl.Error("BadRequest", dsl.ErrorResult, "Bad request")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/memberships")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("pageSize")
			dsl.Param("pageToken")
			dsl.Param("sort")
			dsl.Param("filter")
			dsl.Param("search_name")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK)
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("BadRequest", dsl.StatusBadRequest)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("get-project-membership", func() {
		dsl.Description("Get a specific membership by UID within a project")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("membership", ProjectMembershipResponse, "Membership details")
			ETagAttribute()
			dsl.Required("membership")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Membership not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/memberships/{membership_uid}")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK, func() {
				dsl.Body("membership")
				dsl.Header("etag:ETag")
			})
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	// ── Key contacts (Project_Role__c per membership) ────────────────────────

	dsl.Method("list-membership-key-contacts", func() {
		dsl.Description("List key contacts (Project_Role__c records) for a specific membership, with denormalized contact and company attributes")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("contacts", dsl.ArrayOf(ProjectKeyContactResponse), "List of key contacts")
			dsl.Required("contacts")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Membership not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/memberships/{membership_uid}/key_contacts")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK)
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("create-membership-key-contact", func() {
		dsl.Description("Create a new key contact (Project_Role__c record) for a specific membership")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
			dsl.Attribute("email", dsl.String, "Contact email address; used to resolve or create the B2B Salesforce Contact record", func() {
				dsl.Format(dsl.FormatEmail)
				dsl.Example("john.doe@example.com")
			})
			dsl.Attribute("first_name", dsl.String, "Contact first name; used when creating a new Contact on miss", func() {
				dsl.Example("John")
			})
			dsl.Attribute("last_name", dsl.String, "Contact last name; used when creating a new Contact on miss", func() {
				dsl.Example("Doe")
			})
			dsl.Attribute("title", dsl.String, "Contact job title; used when creating a new Contact on miss", func() {
				dsl.Example("CTO")
			})
			dsl.Attribute("role", dsl.String, "Contact role designation, e.g. 'Voting Representative'", func() {
				dsl.Example("Voting Representative")
			})
			dsl.Attribute("status", dsl.String, "Role record status, e.g. 'Active'", func() {
				dsl.Example("Active")
			})
			dsl.Attribute("board_member", dsl.Boolean, "Whether this contact holds a board member role", func() {
				dsl.Example(false)
			})
			dsl.Attribute("primary_contact", dsl.Boolean, "Whether this is the primary contact for the membership", func() {
				dsl.Example(false)
			})
			dsl.Required("email", "first_name", "last_name")
		})

		dsl.Result(func() {
			dsl.Attribute("contact", ProjectKeyContactResponse, "Newly created key contact")
			dsl.Required("contact")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Membership not found")
		dsl.Error("BadRequest", dsl.ErrorResult, "Bad request")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.POST("/projects/{project_uid}/memberships/{membership_uid}/key_contacts")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusCreated, func() {
				dsl.Body("contact")
			})
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("BadRequest", dsl.StatusBadRequest)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("update-membership-key-contact", func() {
		dsl.Description("Update a key contact (Project_Role__c record) within a membership")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
			ContactUIDAttribute()
			dsl.Attribute("role", dsl.String, "Contact role designation, e.g. 'Voting Representative'", func() {
				dsl.Example("Voting Representative")
			})
			dsl.Attribute("status", dsl.String, "Role record status, e.g. 'Active'", func() {
				dsl.Example("Active")
			})
			dsl.Attribute("board_member", dsl.Boolean, "Whether this contact holds a board member role", func() {
				dsl.Example(false)
			})
			dsl.Attribute("primary_contact", dsl.Boolean, "Whether this is the primary contact for the membership", func() {
				dsl.Example(false)
			})
		})

		dsl.Result(func() {
			dsl.Attribute("contact", ProjectKeyContactResponse, "Updated key contact")
			dsl.Required("contact")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Key contact not found")
		dsl.Error("BadRequest", dsl.ErrorResult, "Bad request")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.PUT("/projects/{project_uid}/memberships/{membership_uid}/key_contacts/{contact_uid}")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Param("contact_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK, func() {
				dsl.Body("contact")
			})
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("BadRequest", dsl.StatusBadRequest)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("delete-membership-key-contact", func() {
		dsl.Description("Delete a key contact (Project_Role__c record) from a membership")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
			ContactUIDAttribute()
		})

		dsl.Result(dsl.Empty)

		dsl.Error("NotFound", dsl.ErrorResult, "Key contact not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.DELETE("/projects/{project_uid}/memberships/{membership_uid}/key_contacts/{contact_uid}")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Param("contact_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusNoContent)
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("get-membership-key-contact", func() {
		dsl.Description("Get a specific key contact by UID within a membership")

		dsl.Security(JWTAuth)

		dsl.Payload(func() {
			BearerTokenAttribute()
			VersionAttribute()
			ProjectUIDAttribute()
			MembershipUIDAttribute()
			ContactUIDAttribute()
		})

		dsl.Result(func() {
			dsl.Attribute("contact", ProjectKeyContactResponse, "Key contact details")
			dsl.Required("contact")
		})

		dsl.Error("NotFound", dsl.ErrorResult, "Key contact not found")
		dsl.Error("InternalServerError", dsl.ErrorResult, "Internal server error", func() { dsl.Fault() })
		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/projects/{project_uid}/memberships/{membership_uid}/key_contacts/{contact_uid}")
			dsl.Param("version:v")
			dsl.Param("project_uid")
			dsl.Param("membership_uid")
			dsl.Param("contact_uid")
			dsl.Header("bearer_token:Authorization")
			dsl.Response(dsl.StatusOK, func() {
				dsl.Body("contact")
			})
			dsl.Response("NotFound", dsl.StatusNotFound)
			dsl.Response("InternalServerError", dsl.StatusInternalServerError)
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	// ── Health checks ────────────────────────────────────────────────────────

	dsl.Method("readyz", func() {
		dsl.Description("Check if the service is able to take inbound requests.")
		dsl.Meta("swagger:generate", "false")
		dsl.Result(dsl.Bytes, func() {
			dsl.Example("OK")
		})

		dsl.Error("ServiceUnavailable", dsl.ErrorResult, "Service unavailable", func() { dsl.Temporary() })

		dsl.HTTP(func() {
			dsl.GET("/readyz")
			dsl.Response(dsl.StatusOK, func() {
				dsl.ContentType("text/plain")
			})
			dsl.Response("ServiceUnavailable", dsl.StatusServiceUnavailable)
		})
	})

	dsl.Method("livez", func() {
		dsl.Description("Check if the service is alive.")
		dsl.Meta("swagger:generate", "false")
		dsl.Result(dsl.Bytes, func() {
			dsl.Example("OK")
		})
		dsl.HTTP(func() {
			dsl.GET("/livez")
			dsl.Response(dsl.StatusOK, func() {
				dsl.ContentType("text/plain")
			})
		})
	})

	dsl.Method("debug-vars", func() {
		dsl.Description("Expose expvar debug variables as JSON. Accessible via kubectl port-forward; not exposed by ingress.")
		dsl.Meta("swagger:generate", "false")
		dsl.Result(dsl.Bytes)
		dsl.HTTP(func() {
			dsl.GET("/debug/vars")
			dsl.Response(dsl.StatusOK, func() {
				dsl.ContentType("text/plain")
			})
		})
	})

	// ── OpenAPI spec files ────────────────────────────────────────────────────

	dsl.Files("/_memberships/openapi.json", "gen/http/openapi.json", func() {
		dsl.Meta("swagger:generate", "false")
	})
	dsl.Files("/_memberships/openapi.yaml", "gen/http/openapi.yaml", func() {
		dsl.Meta("swagger:generate", "false")
	})
	dsl.Files("/_memberships/openapi3.json", "gen/http/openapi3.json", func() {
		dsl.Meta("swagger:generate", "false")
	})
	dsl.Files("/_memberships/openapi3.yaml", "gen/http/openapi3.yaml", func() {
		dsl.Meta("swagger:generate", "false")
	})
})
