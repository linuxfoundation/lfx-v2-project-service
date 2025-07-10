// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT
package design

import (
	. "goa.design/goa/v3/dsl"
)

var Project = Type("Project", func() {
	Description("A representation of LFX Projects.")

	Attribute("id", String, "Project ID -- v2 id, not related to v1 id directly", func() {
		Example("123")
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
})

var BadRequestError = Type("BadRequestError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("400")
	})
	Attribute("message", String, "Error message", func() {
		Example("The request was invalid.")
	})
	Required("code", "message")
})

var NotFoundError = Type("NotFoundError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("404")
	})
	Attribute("message", String, "Error message", func() {
		Example("The resource was not found.")
	})
	Required("code", "message")
})

var ConflictError = Type("ConflictError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("409")
	})
	Attribute("message", String, "Error message", func() {
		Example("The resource already exists.")
	})
	Required("code", "message")
})

var InternalServerError = Type("InternalServerError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("500")
	})
	Attribute("message", String, "Error message", func() {
		Example("An internal server error occurred.")
	})
	Required("code", "message")
})

var ServiceUnavailableError = Type("ServiceUnavailableError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("503")
	})
	Attribute("message", String, "Error message", func() {
		Example("The service is unavailable.")
	})
	Required("code", "message")
})
