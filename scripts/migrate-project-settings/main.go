// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	natsmsg "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// OldProjectSettings represents the old format with string arrays
type OldProjectSettings struct {
	UID                 string     `json:"uid"`
	MissionStatement    string     `json:"mission_statement"`
	AnnouncementDate    *time.Time `json:"announcement_date"`
	Auditors            []string   `json:"auditors"`  // Old format: array of strings
	Writers             []string   `json:"writers"`   // Old format: array of strings
	MeetingCoordinators []string   `json:"meeting_coordinators"`
	CreatedAt           *time.Time `json:"created_at"`
	UpdatedAt           *time.Time `json:"updated_at"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <project-uid>\n", os.Args[0])
		os.Exit(1)
	}

	projectUID := os.Args[1]

	// Get NATS URL from environment or use default
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		slog.Error("Failed to create JetStream context", "error", err)
		os.Exit(1)
	}

	// Get the project-settings KV store
	kv, err := js.KeyValue(context.Background(), constants.KVStoreNameProjectSettings)
	if err != nil {
		slog.Error("Failed to get project-settings KV store", "error", err)
		os.Exit(1)
	}

	// Get current project settings
	entry, err := kv.Get(context.Background(), projectUID)
	if err != nil {
		slog.Error("Failed to get project settings", "uid", projectUID, "error", err)
		os.Exit(1)
	}

	// Try to unmarshal as new format first
	var newSettings models.ProjectSettings
	if err := json.Unmarshal(entry.Value(), &newSettings); err == nil {
		// Check if it's already in new format (has UserInfo structs)
		if len(newSettings.Auditors) > 0 || len(newSettings.Writers) > 0 {
			// Check if first auditor/writer has the UserInfo structure
			if len(newSettings.Auditors) > 0 && newSettings.Auditors[0].Name != "" {
				fmt.Printf("Project %s is already in new format\n", projectUID)
				return
			}
			if len(newSettings.Writers) > 0 && newSettings.Writers[0].Name != "" {
				fmt.Printf("Project %s is already in new format\n", projectUID)
				return
			}
		}
	}

	// Try to unmarshal as old format
	var oldSettings OldProjectSettings
	if err := json.Unmarshal(entry.Value(), &oldSettings); err != nil {
		slog.Error("Failed to unmarshal project settings", "uid", projectUID, "error", err)
		os.Exit(1)
	}

	fmt.Printf("Found project settings for %s in old format\n", projectUID)
	fmt.Printf("Writers: %v\n", oldSettings.Writers)
	fmt.Printf("Auditors: %v\n", oldSettings.Auditors)

	// Convert to new format
	newSettings = models.ProjectSettings{
		UID:                 oldSettings.UID,
		MissionStatement:    oldSettings.MissionStatement,
		AnnouncementDate:    oldSettings.AnnouncementDate,
		Auditors:            []models.UserInfo{},
		Writers:             []models.UserInfo{},
		MeetingCoordinators: oldSettings.MeetingCoordinators,
		CreatedAt:           oldSettings.CreatedAt,
		UpdatedAt:           oldSettings.UpdatedAt,
	}

	reader := bufio.NewReader(os.Stdin)

	// Convert auditors
	for _, auditorStr := range oldSettings.Auditors {
		if auditorStr == "" {
			continue
		}
		
		fmt.Printf("\nMigrating auditor: %s\n", auditorStr)
		userInfo := getUserInfo(reader, auditorStr)
		newSettings.Auditors = append(newSettings.Auditors, userInfo)
	}

	// Convert writers
	for _, writerStr := range oldSettings.Writers {
		if writerStr == "" {
			continue
		}
		
		fmt.Printf("\nMigrating writer: %s\n", writerStr)
		userInfo := getUserInfo(reader, writerStr)
		newSettings.Writers = append(newSettings.Writers, userInfo)
	}

	// Update timestamp
	now := time.Now()
	newSettings.UpdatedAt = &now

	// Marshal new settings
	newSettingsBytes, err := json.Marshal(newSettings)
	if err != nil {
		slog.Error("Failed to marshal new settings", "error", err)
		os.Exit(1)
	}

	// Update in NATS KV store
	_, err = kv.Put(context.Background(), projectUID, newSettingsBytes)
	if err != nil {
		slog.Error("Failed to update project settings in KV store", "error", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccessfully updated project settings for %s\n", projectUID)

	// Send indexer sync message
	ctx := context.Background()
	messageBuilder := &natsmsg.MessageBuilder{
		NatsConn: nc,
	}

	// Create indexer message
	indexerMessage := models.ProjectSettingsIndexerMessage{
		Action: models.ActionUpdated,
		Data:   newSettings,
		Tags:   newSettings.Tags(),
	}

	if err := messageBuilder.PublishIndexerMessage(ctx, constants.IndexProjectSettingsSubject, indexerMessage); err != nil {
		slog.Error("Failed to send indexer sync message", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Sent indexer sync message for project %s\n", projectUID)
}

func getUserInfo(reader *bufio.Reader, defaultUsername string) models.UserInfo {
	fmt.Printf("Enter details for user '%s':\n", defaultUsername)
	
	name := getInput(reader, "Name")
	username := getInputWithDefault(reader, "Username", defaultUsername)
	email := getInput(reader, "Email")
	avatar := getInputOptional(reader, "Avatar URL (optional)")

	return models.UserInfo{
		Name:     name,
		Username: username,
		Email:    email,
		Avatar:   avatar,
	}
}

func getInput(reader *bufio.Reader, prompt string) string {
	for {
		fmt.Printf("%s: ", prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}
		
		input = strings.TrimSpace(input)
		if input != "" {
			return input
		}
		
		fmt.Println("This field is required. Please enter a value.")
	}
}

func getInputWithDefault(reader *bufio.Reader, prompt, defaultValue string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue
	}
	
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	
	return input
}

func getInputOptional(reader *bufio.Reader, prompt string) string {
	fmt.Printf("%s: ", prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return ""
	}
	
	return strings.TrimSpace(input)
}