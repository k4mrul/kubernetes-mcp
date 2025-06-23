// Environment variables used by this tool:
// Required:
//   GOOGLE_APPLICATION_CREDENTIALS - Path to the GCP service account JSON file (for local/outside GCP)
// Optional:
//   GOOGLE_CLOUD_PROJECT           - GCP Project ID (used if not provided in input)

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListGCPSecretInput represents the input for listing secrets.
type ListGCPSecretInput struct {
	ProjectID string `json:"projectId,omitempty"`
}

type ListGCPSecretTool struct{}

func NewListGCPSecretTool() *ListGCPSecretTool {
	return &ListGCPSecretTool{}
}

func (t *ListGCPSecretTool) Tool() mcp.Tool {
	return mcp.NewTool("list_gcp_secret",
		mcp.WithDescription("List all secrets in Google Cloud Secret Manager for a project."),
		mcp.WithString("projectId", mcp.Description("GCP Project ID (optional, will use GOOGLE_CLOUD_PROJECT env if not set)")),
	)
}

func (t *ListGCPSecretTool) Handler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// input, err := parseAndValidateListGCPSecretParams(req.Params.Arguments)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to parse input: %w", err)
	// }

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return nil, fmt.Errorf("google credentials not found: set GOOGLE_APPLICATION_CREDENTIALS to a service account JSON file")
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	var secrets []map[string]any

	// Only list the secret dokan-dev-staging-secrets
	secretName := os.Getenv("GCP_SECRET_NAME")
	secretFullName := fmt.Sprintf("projects/%s/secrets/%s", projectID, secretName)
	// Get the latest version's content
	latestVersion := fmt.Sprintf("%s/versions/latest", secretFullName)
	accessReq := &secretmanagerpb.AccessSecretVersionRequest{Name: latestVersion}
	result, err := client.AccessSecretVersion(ctx, accessReq)
	var payload string
	if err == nil && result != nil && result.Payload != nil {
		payload = string(result.Payload.Data)
	} else {
		payload = "(unable to fetch latest version or secret is empty)"
	}
	secrets = append(secrets, map[string]any{
		"name":    secretFullName,
		"payload": payload,
	})

	// _ = secrets // Uncomment if you want to include secrets in the output
	output := map[string]any{
		// "projectId": input.ProjectID,
		"secrets": secrets,
		"status":  "Listed all secrets in the project",
	}
	out, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

// func parseAndValidateListGCPSecretParams(args map[string]any) (*ListGCPSecretInput, error) {
// 	input := &ListGCPSecretInput{}
// 	if v, ok := args["projectId"]; ok && v != nil {
// 		input.ProjectID = v.(string)
// 	}
// 	return input, nil
// }
