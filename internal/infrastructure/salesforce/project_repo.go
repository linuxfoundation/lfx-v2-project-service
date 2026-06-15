// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	sf "github.com/k-capehart/go-salesforce/v3"
)

// projectBySlugSOQL fetches a single Project__c record by its Slug__c value.
// The caller must substitute a quoteSOQL-escaped slug for the %s placeholder.
const projectBySlugSOQL = `
SELECT Id, Name, Slug__c
FROM Project__c
WHERE Slug__c = %s
    AND IsDeleted = false
`

// projectByIDSOQL fetches a single Project__c record by its Salesforce ID.
// The caller must substitute a quoteSOQL-escaped ID for the %s placeholder.
const projectByIDSOQL = `
SELECT Id, Name, Slug__c
FROM Project__c
WHERE Id = %s
`

// projectsByIDsSOQL fetches multiple Project__c records by a set of Salesforce
// IDs. The caller must substitute a comma-separated, quoteSOQL-escaped list
// enclosed in parentheses for the %s placeholder (e.g. "('id1','id2')").
const projectsByIDsSOQL = `
SELECT Id, Name, Slug__c
FROM Project__c
WHERE Id IN %s
`

// ProjectRepo handles Salesforce SOQL queries for project ID mappings.
type ProjectRepo struct {
	client *sf.Salesforce
}

// NewProjectRepo creates a new ProjectRepo backed by the given Salesforce client.
func NewProjectRepo(client *sf.Salesforce) *ProjectRepo {
	return &ProjectRepo{client: client}
}

// FetchProjectByID fetches a single Project__c record by its Salesforce ID.
// Returns nil if the project is not found.
func (r *ProjectRepo) FetchProjectByID(ctx context.Context, sfid string) (*soqlProject, error) {
	slog.DebugContext(ctx, "fetching project from Salesforce by ID", "sfid", sfid)

	var projects []soqlProject
	if err := r.client.Query(fmt.Sprintf(projectByIDSOQL, quoteSOQL(sfid)), &projects); err != nil {
		return nil, fmt.Errorf("fetching project by ID %s: %w", sfid, err)
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &projects[0], nil
}

// FetchSFIDBySlug returns the Salesforce Project__c.Id for the given slug.
// Returns an empty string (not an error) if no matching project is found.
// Returns an error if more than one project matches the slug, as the slug
// must be unique; ambiguous results are treated as a data integrity error.
func (r *ProjectRepo) FetchSFIDBySlug(ctx context.Context, slug string) (string, error) {
	slog.DebugContext(ctx, "fetching project from Salesforce by slug", "slug", slug)

	var projects []soqlProject
	if err := r.client.Query(fmt.Sprintf(projectBySlugSOQL, quoteSOQL(slug)), &projects); err != nil {
		return "", fmt.Errorf("fetching project by slug %s: %w", slug, err)
	}

	switch len(projects) {
	case 0:
		return "", nil
	case 1:
		return normalizeUID("Project__c", projects[0].ID)
	default:
		return "", fmt.Errorf("slug %q matched %d Project__c records; expected at most 1", slug, len(projects))
	}
}

// soqlBatchSize is the maximum number of IDs in a single SOQL IN clause.
// Salesforce caps SOQL query length; 200 IDs per batch is a safe ceiling.
const soqlBatchSize = 200

// FetchProjectsByIDs fetches multiple Project__c records by their Salesforce IDs
// in a single batch query. Splits into chunks of soqlBatchSize to respect SOQL
// query-length limits. Returns an empty slice when none are found.
func (r *ProjectRepo) FetchProjectsByIDs(ctx context.Context, sfids []string) ([]*soqlProject, error) {
	if len(sfids) == 0 {
		return nil, nil
	}
	slog.DebugContext(ctx, "fetching projects from Salesforce by IDs (batch)", "count", len(sfids))

	all := make([]*soqlProject, 0, len(sfids))
	for start := 0; start < len(sfids); start += soqlBatchSize {
		end := start + soqlBatchSize
		if end > len(sfids) {
			end = len(sfids)
		}
		chunk := sfids[start:end]

		quoted := make([]string, len(chunk))
		for i, id := range chunk {
			quoted[i] = quoteSOQL(id)
		}
		inClause := "(" + strings.Join(quoted, ",") + ")"

		var projects []soqlProject
		if err := r.client.Query(fmt.Sprintf(projectsByIDsSOQL, inClause), &projects); err != nil {
			return nil, fmt.Errorf("batch fetching projects by IDs: %w", err)
		}
		for i := range projects {
			all = append(all, &projects[i])
		}
	}
	return all, nil
}
