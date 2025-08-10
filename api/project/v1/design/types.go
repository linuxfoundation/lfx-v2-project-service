// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

// BearerTokenAttribute is a reusable token attribute for JWT authentication.
func BearerTokenAttribute() {
	Token("bearer_token", String, func() {
		Description("JWT token issued by Heimdall")
		Example("eyJhbGci...")
	})
}

// EtagAttribute is a reusable ETag header attribute.
func EtagAttribute() {
	Attribute("etag", String, "ETag header value", func() {
		Example("123")
	})
}

// VersionAttribute is a reusable version attribute.
func VersionAttribute() {
	Attribute("version", String, "Version of the API", func() {
		Enum("1")
		Example("1")
	})
}

// ProjectFull is the DSL type for a project full.
var ProjectFull = Type("ProjectFull", func() {
	Description("A full representation of LF Projects with sub-objects populated.")

	ProjectBaseAttributes()
	ProjectSettingsAttributes()
})

// ProjectBase is the DSL type for a project base.
var ProjectBase = Type("ProjectBase", func() {
	Description("A base representation of LF Projects without sub-objects.")

	ProjectBaseAttributes()
})

// ProjectBaseAttributes is the DSL attributes for a project base.
func ProjectBaseAttributes() {
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
	ProjectCreatedAtAttribute()
	ProjectUpdatedAtAttribute()
}

// ProjectSettings is the DSL type for a project settings.
var ProjectSettings = Type("ProjectSettings", func() {
	Description("A representation of LF Project settings.")

	ProjectSettingsAttributes()
})

// ProjectSettingsAttributes is the DSL attributes for a project settings.
func ProjectSettingsAttributes() {
	ProjectUIDAttribute()
	ProjectMissionStatementAttribute()
	ProjectAnnouncementDateAttribute()
	ProjectWritersAttribute()
	ProjectMeetingCoordinatorsAttribute()
	ProjectAuditorsAttribute()
	ProjectCreatedAtAttribute()
	ProjectUpdatedAtAttribute()
}

//
// Project attributes
//

// ProjectUIDAttribute is the DSL attribute for a project UID.
func ProjectUIDAttribute() {
	Attribute("uid", String, "Project UID -- v2 uid, not related to v1 id directly", func() {
		// Read-only attribute
		Example("7cad5a8d-19d0-41a4-81a6-043453daf9ee")
		Format(FormatUUID)
	})
}

// ProjectSlugAttribute is the DSL attribute for a project slug.
func ProjectSlugAttribute() {
	Attribute("slug", String, "Project slug, a short slugified name of the project", func() {
		// Write-once attribute
		Example("project-slug")
		Format(FormatRegexp)
		Pattern(`^[a-z][a-z0-9_\-]*[a-z0-9]$`)
	})
}

// ProjectNameAttribute is the DSL attribute for a project name.
func ProjectNameAttribute() {
	Attribute("name", String, "The pretty name of the project", func() {
		Example("Foo Foundation")
	})
}

// ProjectDescriptionAttribute is the DSL attribute for a project description.
func ProjectDescriptionAttribute() {
	Attribute("description", String, "A description of the project", func() {
		Example("project foo is a project about bar")
	})
}

// ProjectStageAttribute is the DSL attribute for a project stage.
func ProjectStageAttribute() {
	Attribute("stage", String, "The stage of the project", func() {
		Example("Formation - Exploratory")
		Enum(
			"Formation - Exploratory",
			"Formation - Engaged",
			"Active",
			"Archived",
			"Formation - On Hold",
			"Formation - Disengaged",
			"Formation - Confidential",
			"Prospect",
		)
	})
}

// ProjectCategoryAttribute is the DSL attribute for a project category.
func ProjectCategoryAttribute() {
	Attribute("category", String, "The category of the project", func() {
		Example("Active")
		Enum(
			"Active",
			"Adopted",
			"Archived",
			"At-Large",
			"Early Adoption",
			"Emeritus",
			"Graduated",
			"Growth",
			"Idle",
			"Impact",
			"Incubating",
			"Kanister",
			"Mature",
			"Pre-LFESS",
			"Sandbox",
			"SIG",
			"Standards",
			"TAC",
			"Working Group",
			"TAG",
			"NONE",
		)
	})
}

// ProjectPublicAttribute is the DSL attribute for a project public flag.
func ProjectPublicAttribute() {
	Attribute("public", Boolean, "Whether the project is public", func() {
		Example(true)
	})
}

// ProjectParentUIDAttribute is the DSL attribute for a project parent UID.
func ProjectParentUIDAttribute() {
	Attribute("parent_uid", String, "The UID of the parent project, required and must be a valid UUID", func() {
		// Format(FormatUUID) is not included here to allow server-side validation
		// Server code validates this is a non-empty valid UUID of an existing project
		Example("7cad5a8d-19d0-41a4-81a6-043453daf9ee")
	})
}

// ProjectLogoURLAttribute is the DSL attribute for a project logo URL.
func ProjectLogoURLAttribute() {
	Attribute("logo_url", String, "The URL of the project logo", func() {
		Example("https://example.com/logo.png")
		Format(FormatURI)
	})
}

// ProjectWebsiteURLAttribute is the DSL attribute for a project website URL.
func ProjectWebsiteURLAttribute() {
	Attribute("website_url", String, "The URL of the project website", func() {
		Example("https://example.com")
		Format(FormatURI)
	})
}

// ProjectRepositoryURLAttribute is the DSL attribute for a project repository URL.
func ProjectRepositoryURLAttribute() {
	Attribute("repository_url", String, "The URL of the project repository", func() {
		Example("https://example.com/project")
		Format(FormatURI)
	})
}

// ProjectAuditorsAttribute is the DSL attribute for a project auditors.
func ProjectAuditorsAttribute() {
	Attribute("auditors", ArrayOf(String), "A list of project auditors by their user IDs", func() {
		Example([]string{"user123", "user456"})
	})
}

// ProjectWritersAttribute is the DSL attribute for a project writers.
func ProjectWritersAttribute() {
	Attribute("writers", ArrayOf(String), "A list of project writers by their user IDs", func() {
		Example([]string{"user123", "user456"})
	})
}

// ProjectMeetingCoordinatorsAttribute is the DSL attribute for a project meeting coordinators.
func ProjectMeetingCoordinatorsAttribute() {
	Attribute("meeting_coordinators", ArrayOf(String), func() {
		Description(
			"A list of project meeting coordinators by their user IDs. " +
				"These users are responsible for managing meetings for the project.",
		)
		Example([]string{"user123", "user456"})
	})
}

// ProjectLegalEntityTypeAttribute is the DSL attribute for a project legal entity type.
func ProjectLegalEntityTypeAttribute() {
	Attribute("legal_entity_type", String, "The legal entity type of the project", func() {
		Example("Subproject")
		Enum(
			"Subproject",
			"Incorporated Entity",
			"Series LLC",
			"Unofficial Subproject",
			"Internal Allocation",
			"None",
		)
	})
}

// ProjectLegalEntityNameAttribute is the DSL attribute for a project legal entity name.
func ProjectLegalEntityNameAttribute() {
	Attribute("legal_entity_name", String, "The legal entity name of the project", func() {
		Example("Example Foundation LLC")
	})
}

// ProjectLegalParentUIDAttribute is the DSL attribute for a project legal parent UID.
func ProjectLegalParentUIDAttribute() {
	Attribute("legal_parent_uid", String, "The UID of the legal parent entity, should be empty if there is none", func() {
		Example("7cad5a8d-19d0-41a4-81a6-043453daf9ee")
		Format(FormatUUID)
	})
}

// ProjectFundingModelAttribute is the DSL attribute for a project funding model.
func ProjectFundingModelAttribute() {
	Attribute("funding_model", ArrayOf(String), "A list of funding models for the project", func() {
		Example([]string{"Crowdfunding"})
		// Each string element must be one of the enum values
		Elem(func() {
			Enum(
				"Crowdfunding",
				"Membership",
				"Alternate Funding",
			)
		})
	})
}

// ProjectCharterURLAttribute is the DSL attribute for a project charter URL.
func ProjectCharterURLAttribute() {
	Attribute("charter_url", String, "The URL of the project charter document", func() {
		Example("https://example.com/charter.pdf")
		Format(FormatURI)
	})
}

// ProjectEntityDissolutionDateAttribute is the DSL attribute for a project entity dissolution date.
func ProjectEntityDissolutionDateAttribute() {
	Attribute("entity_dissolution_date", String, "The date the project entity was dissolved", func() {
		Example("2021-12-31")
		Format(FormatDate)
	})
}

// ProjectEntityFormationDocumentURLAttribute is the DSL attribute for a project entity formation document URL.
func ProjectEntityFormationDocumentURLAttribute() {
	Attribute("entity_formation_document_url", String, "The URL of the project entity formation document", func() {
		Example("https://example.com/formation.pdf")
		Format(FormatURI)
	})
}

// ProjectAutojoinEnabledAttribute is the DSL attribute for a project autojoin enabled flag.
func ProjectAutojoinEnabledAttribute() {
	Attribute("autojoin_enabled", Boolean, "Whether autojoin is enabled for the project", func() {
		Example(false)
	})
}

// ProjectFormationDateAttribute is the DSL attribute for a project formation date.
func ProjectFormationDateAttribute() {
	Attribute("formation_date", String, "The date the project was formed", func() {
		Example("2021-01-01")
		Format(FormatDate)
	})
}

// ProjectAnnouncementDateAttribute is the DSL attribute for a project announcement date.
func ProjectAnnouncementDateAttribute() {
	Attribute("announcement_date", String, "The date the project was announced", func() {
		Example("2021-01-01")
		Format(FormatDate)
	})
}

// ProjectCreatedAtAttribute is the DSL attribute for a project created at timestamp.
func ProjectCreatedAtAttribute() {
	Attribute("created_at", String, "The date and time the project was created", func() {
		// Read-only attribute
		Example("2021-01-01T00:00:00Z")
		Format(FormatDateTime)
	})
}

// ProjectUpdatedAtAttribute is the DSL attribute for a project updated at timestamp.
func ProjectUpdatedAtAttribute() {
	Attribute("updated_at", String, "The date and time the project was last updated", func() {
		// Read-only attribute
		Example("2021-01-01T00:00:00Z")
		Format(FormatDateTime)
	})
}

// ProjectMissionStatementAttribute is the DSL attribute for a project mission statement.
func ProjectMissionStatementAttribute() {
	Attribute("mission_statement", String, "The mission statement of the project", func() {
		Example("The mission of the project is to build a sustainable ecosystem around open source projects to accelerate technology development and industry adoption.")
	})
}

//
// Error types
//

// BadRequestError is the DSL type for a bad request error.
var BadRequestError = Type("BadRequestError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("400")
	})
	Attribute("message", String, "Error message", func() {
		Example("The request was invalid.")
	})
	Required("code", "message")
})

// NotFoundError is the DSL type for a not found error.
var NotFoundError = Type("NotFoundError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("404")
	})
	Attribute("message", String, "Error message", func() {
		Example("The resource was not found.")
	})
	Required("code", "message")
})

// ConflictError is the DSL type for a conflict error.
var ConflictError = Type("ConflictError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("409")
	})
	Attribute("message", String, "Error message", func() {
		Example("The resource already exists.")
	})
	Required("code", "message")
})

// InternalServerError is the DSL type for an internal server error.
var InternalServerError = Type("InternalServerError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("500")
	})
	Attribute("message", String, "Error message", func() {
		Example("An internal server error occurred.")
	})
	Required("code", "message")
})

// ServiceUnavailableError is the DSL type for a service unavailable error.
var ServiceUnavailableError = Type("ServiceUnavailableError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("503")
	})
	Attribute("message", String, "Error message", func() {
		Example("The service is unavailable.")
	})
	Required("code", "message")
})
