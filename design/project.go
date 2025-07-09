package design

import (
	. "goa.design/goa/v3/dsl"
)

var JWTAuth = JWTSecurity("jwt", func() {
	Description("Heimdall authorization")
})

var _ = Service("project-service", func() {
	Description("The project service provides LFX Project resources.")

	Method("get-projects", func() {
		Description("Get all projects.")

		//Security(JWTAuth)

		Payload(func() {
			//Token("bearer_token", String, func() {
			//	Description("JWT token issued by Heimdall")
			//	Example("eyJhbGci...")
			//})
			Attribute("version", String, "Version of the API", func() {
				Enum("1")
				Example("1")
			})
			Attribute("page_token", String, "opaque page token", func() {
				Description("Token to get the next page of results, if available")
				Example("****")
			})
			//Required("bearer_token", "version")
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

		Error("BadRequest", ErrorResult, "Bad request")

		HTTP(func() {
			GET("/projects")
			Param("version:v")
			Param("page_token")
			//Header("bearer_token:Authorization")
			Response(StatusOK, func() {
				Header("cache_control:Cache-Control")
			})
			Response("BadRequest", StatusBadRequest)
		})
	})

	Method("readyz", func() {
		Description("Check if the service is able to take inbound requests.")
		Result(Bytes, func() {
			Example("OK")
		})
		Error("NotReady", func() {
			Description("Service is not ready yet")
			Temporary()
			Fault()
		})
		HTTP(func() {
			GET("readyz")
			Response(StatusOK, func() {
				ContentType("text/plain")
			})
			Response("NotReady", StatusServiceUnavailable)
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
