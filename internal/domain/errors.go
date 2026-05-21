// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "errors"

// Domain errors
var (
	// ErrProjectNotFound is returned when a project is not found.
	ErrProjectNotFound = errors.New("project not found")
	// ErrInvalidParentProject is returned when a parent project is invalid.
	ErrInvalidParentProject = errors.New("invalid parent project")
	// ErrProjectSlugExists is returned when a project slug already exists.
	ErrProjectSlugExists = errors.New("project slug already exists")
	// ErrInternal is returned when an internal error occurs.
	ErrInternal = errors.New("internal error")
	// ErrRevisionMismatch is returned when a revision mismatch occurs.
	ErrRevisionMismatch = errors.New("revision mismatch")
	// ErrUnmarshal is returned when an unmarshal error occurs.
	ErrUnmarshal = errors.New("unmarshal error")
	// ErrServiceUnavailable is returned when a service is unavailable.
	ErrServiceUnavailable = errors.New("service unavailable")
	// ErrValidationFailed is returned when a validation failed.
	ErrValidationFailed = errors.New("validation failed")
	// ErrCannotDeleteNonCrowdfundingProject is returned when attempting to delete a project whose funding model is not exactly ["Crowdfunding"].
	ErrCannotDeleteNonCrowdfundingProject = errors.New("project can only be deleted if its funding model is Crowdfunding only")

	// ErrDocumentNotFound is returned when a document is not found.
	ErrDocumentNotFound = errors.New("document not found")
	// ErrDocumentNameExists is returned when a document with the same name already exists.
	ErrDocumentNameExists = errors.New("document with the same name already exists for this project")
	// ErrInvalidContentType is returned when the uploaded file has a disallowed MIME type.
	ErrInvalidContentType = errors.New("content type is not allowed")
	// ErrFileTooLarge is returned when the uploaded file exceeds the maximum allowed size.
	ErrFileTooLarge = errors.New("file size exceeds maximum allowed size")

	// ErrLinkNotFound is returned when a link is not found.
	ErrLinkNotFound = errors.New("link not found")

	// ErrFolderNotFound is returned when a folder is not found.
	ErrFolderNotFound = errors.New("folder not found")
	// ErrFolderNameExists is returned when a folder with the same name already exists.
	ErrFolderNameExists = errors.New("folder with the same name already exists for this project")
	// ErrFolderNotEmpty is returned when attempting to delete a folder that still contains links or documents.
	ErrFolderNotEmpty = errors.New("folder cannot be deleted because it contains links or documents; remove all items from the folder first")

	// ErrInviteMappingNotFound is returned when no invite→project mapping exists for the given invite UID.
	// This is distinct from ErrProjectNotFound: the project may exist but no mapping was written for this invite.
	ErrInviteMappingNotFound = errors.New("invite mapping not found")
)
