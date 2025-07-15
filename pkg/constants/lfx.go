// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package constants

// LFXEnvironment is the environment name of the LFX platform.
type LFXEnvironment string

// Constants for the environment names of the LFX platform.
const (
	LFXEnvironmentDev  LFXEnvironment = "dev"
	LFXEnvironmentStg  LFXEnvironment = "stg"
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
