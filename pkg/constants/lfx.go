// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// LFXEnvironment is the environment name of the LFX platform.
type LFXEnvironment string

// Constants for the environment names of the LFX platform.
const (
	// LFXEnvironmentDev is the development environment name of the LFX platform.
	LFXEnvironmentDev LFXEnvironment = "dev"
	// LFXEnvironmentStg is the staging environment name of the LFX platform.
	LFXEnvironmentStg LFXEnvironment = "stg"
	// LFXEnvironmentProd is the production environment name of the LFX platform.
	LFXEnvironmentProd LFXEnvironment = "prod"
)

// ParseLFXEnvironment parses the LFX environment from a string.
func ParseLFXEnvironment(env string) LFXEnvironment {
	switch env {
	case "dev", "development":
		return LFXEnvironmentDev
	case "stg", "stage", "staging":
		return LFXEnvironmentStg
	case "prod", "production":
		return LFXEnvironmentProd
	default:
		return LFXEnvironmentDev
	}
}
