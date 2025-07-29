// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package design

import (
	//nolint:staticcheck // ST1001: the recommended way of using the goa GSL package is with the . import
	. "goa.design/goa/v3/dsl"
)

// Project is the DSL type for a project.
var Project = Type("Project", func() {
	Description("A representation of LFX Projects.")

	// Attributes
	ProjectIDAttribute()
	ProjectSlugAttribute()
	ProjectDescriptionAttribute()
	ProjectNameAttribute()
	ProjectPublicAttribute()
	ProjectParentUIDAttribute()
	ProjectAuditorsAttribute()
	ProjectWritersAttribute()
})

//
// Project attributes
//

// ProjectIDAttribute is the DSL attribute for a project ID.
func ProjectIDAttribute() {
	Attribute("id", String, "Project ID -- v2 id, not related to v1 id directly", func() {
		Example("7cad5a8d-19d0-41a4-81a6-043453daf9ee")
		Format(FormatUUID)
	})
}

// ProjectSlugAttribute is the DSL attribute for a project slug.
func ProjectSlugAttribute() {
	Attribute("slug", String, "Project slug, a short slugified name of the project", func() {
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
