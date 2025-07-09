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

var NotFoundError = Type("NotFoundError", func() {
	Attribute("code", String, "HTTP status code", func() {
		Example("404")
	})
	Attribute("message", String, "Error message", func() {
		Example("The project was not found.")
	})
	Required("code", "message")
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
