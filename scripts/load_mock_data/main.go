// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is the main package for the load_mock_data tool.
// The tool loads mock data into the project service API.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	projectservice "github.com/linuxfoundation/lfx-v2-project-service/cmd/project-api/gen/project_service"
)

// ProjectData represents the structure for creating a project
type ProjectData struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Public      bool     `json:"public"`
	ParentUID   string   `json:"parent_uid"`
	Auditors    []string `json:"auditors"`
	Writers     []string `json:"writers"`
}

// ProjectResponse represents the response from the API
type ProjectResponse struct {
	ID          *string  `json:"id,omitempty"`
	Slug        *string  `json:"slug,omitempty"`
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Public      *bool    `json:"public,omitempty"`
	ParentUID   *string  `json:"parent_uid,omitempty"`
	Auditors    []string `json:"auditors,omitempty"`
	Writers     []string `json:"writers,omitempty"`
}

// Config holds the configuration for the script
type Config struct {
	APIURL      string
	BearerToken string
	NumProjects int
	Version     string
	Timeout     time.Duration
}

// ProjectGenerator generates random project data
type ProjectGenerator struct {
	descriptions []string
	managerIDs   []string
}

// NewProjectGenerator creates a new project generator with predefined data
func NewProjectGenerator() *ProjectGenerator {
	return &ProjectGenerator{
		descriptions: []string{
			"A test description 1",
			"A test description 2",
			"A test description 3",
			"A test description 4",
			"A test description 5",
			"A test description 6",
			"A test description 7",
			"A test description 8",
			"A test description 9",
			"A test description 10",
		},
		managerIDs: []string{
			"user123", "user456", "user789", "admin001", "admin002", "manager001",
			"manager002", "lead001", "lead002", "pm001", "pm002", "tech_lead_001",
		},
	}
}

// GenerateProject creates a random project with the given index
func (pg *ProjectGenerator) GenerateProject(index int) ProjectData {
	// Generate a random project number (1 to 100000)
	projectNumber, _ := rand.Int(rand.Reader, big.NewInt(100000))
	projectNumber.Add(projectNumber, big.NewInt(1)) // Add 1 to make range 1-100000

	// Generate a random description
	descIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pg.descriptions))))

	name := fmt.Sprintf("Project %d", projectNumber.Int64())
	description := pg.descriptions[descIndex.Int64()]

	// Generate slug from name
	slug := generateSlug(name)

	// Generate random public flag
	publicInt, _ := rand.Int(rand.Reader, big.NewInt(2))
	publicInt.Add(publicInt, big.NewInt(1))
	public := publicInt.Int64() == 1

	// Generate random auditors (1-3 auditors)
	numAuditors, _ := rand.Int(rand.Reader, big.NewInt(3))
	numAuditors.Add(numAuditors, big.NewInt(1))

	auditors := make([]string, numAuditors.Int64())
	for i := range auditors {
		auditorIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pg.managerIDs))))
		auditors[i] = pg.managerIDs[auditorIndex.Int64()]
	}

	// Generate random writers (1-3 writers)
	numWriters, _ := rand.Int(rand.Reader, big.NewInt(3))
	numWriters.Add(numWriters, big.NewInt(1))

	writers := make([]string, numWriters.Int64())
	for i := range writers {
		writerIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pg.managerIDs))))
		writers[i] = pg.managerIDs[writerIndex.Int64()]
	}

	return ProjectData{
		Slug:        slug,
		Name:        name,
		Description: description,
		Public:      public,
		ParentUID:   "",
		Auditors:    auditors,
		Writers:     writers,
	}
}

// generateSlug creates a URL-friendly slug from a name
func generateSlug(name string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = strings.ReplaceAll(slug, ".", "")
	slug = strings.ReplaceAll(slug, ",", "")
	slug = strings.ReplaceAll(slug, "(", "")
	slug = strings.ReplaceAll(slug, ")", "")

	// Remove multiple consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Ensure it starts with a letter and add index if needed
	if len(slug) == 0 || !isLetter(slug[0]) {
		slug = "project-" + slug
	}

	return slug
}

// isLetter checks if a byte is a letter
func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// ProjectClient handles API communication
type ProjectClient struct {
	config *Config
	client *http.Client
}

// NewProjectClient creates a new project client
func NewProjectClient(config *Config) *ProjectClient {
	return &ProjectClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// CreateProject sends a project creation request to the API
func (pc *ProjectClient) CreateProject(ctx context.Context, project ProjectData) (*ProjectResponse, error) {
	// Create the payload
	payload := &projectservice.CreateProjectPayload{
		Slug:        project.Slug,
		Name:        project.Name,
		Description: project.Description,
		Public:      &project.Public,
		ParentUID:   &project.ParentUID,
		Auditors:    project.Auditors,
		Writers:     project.Writers,
	}

	// Marshal the payload
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"slug":        payload.Slug,
		"name":        payload.Name,
		"description": payload.Description,
		"public":      payload.Public,
		"parent_uid":  payload.ParentUID,
		"auditors":    payload.Auditors,
		"writers":     payload.Writers,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, "POST", pc.config.APIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pc.config.BearerToken)

	// Add version query parameter
	q := req.URL.Query()
	q.Add("v", pc.config.Version)
	req.URL.RawQuery = q.Encode()

	// Send the request
	resp, err := pc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("request failed with status %d: %v", resp.StatusCode, errorResp)
	}

	// Parse response
	var projectResp ProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&projectResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &projectResp, nil
}

// LoadMockData loads the specified number of projects
func (pc *ProjectClient) LoadMockData(ctx context.Context, numProjects int) error {
	generator := NewProjectGenerator()

	log.Printf("Starting to create %d projects...", numProjects)

	successCount := 0
	errorCount := 0

	for i := 0; i < numProjects; i++ {
		project := generator.GenerateProject(i)

		log.Printf("Creating project %d/%d: %s (%s)", i+1, numProjects, project.Name, project.Slug)

		_, err := pc.CreateProject(ctx, project)
		if err != nil {
			log.Printf("Error creating project %s: %v", project.Slug, err)
			errorCount++
			continue
		}

		log.Printf("Successfully created project: %s", project.Slug)
		successCount++

		// Add a small delay to avoid overwhelming the API
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Completed! Successfully created %d projects, %d errors", successCount, errorCount)
	return nil
}

func main() {
	// Parse command line flags
	var (
		apiURL      = flag.String("api-url", "http://localhost:8080/projects", "Project service API URL")
		bearerToken = flag.String("bearer-token", "", "Bearer token for authentication")
		numProjects = flag.Int("num-projects", 10, "Number of projects to create")
		version     = flag.String("version", "1", "API version")
		timeout     = flag.Duration("timeout", 30*time.Second, "Request timeout")
	)
	flag.Parse()

	// Validate required parameters
	if *numProjects <= 0 {
		log.Fatal("Number of projects must be greater than 0.")
	}

	// Create configuration
	config := &Config{
		APIURL:      *apiURL,
		BearerToken: *bearerToken,
		NumProjects: *numProjects,
		Version:     *version,
		Timeout:     *timeout,
	}

	// Create client
	client := NewProjectClient(config)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout*time.Duration(config.NumProjects))
	defer cancel()

	// Load mock data
	if err := client.LoadMockData(ctx, config.NumProjects); err != nil {
		log.Printf("Failed to load mock data: %v", err)
		return
	}

	log.Println("Mock data loading completed successfully!")
}
