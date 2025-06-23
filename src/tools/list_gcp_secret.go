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
	input, err := parseAndValidateListGCPSecretParams(req.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	if input.ProjectID == "" {
		input.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if input.ProjectID == "" {
		return nil, fmt.Errorf("projectId must be provided (either as input or environment variable)")
	}

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return nil, fmt.Errorf("google credentials not found: set GOOGLE_APPLICATION_CREDENTIALS to a service account JSON file")
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s", input.ProjectID)
	it := client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{Parent: parent})
	var secrets []map[string]any
	for {
		secret, err := it.Next()
		if err != nil {
			if err.Error() == "iterator done" || err.Error() == "no more items in iterator" {
				break
			}
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}

		// Get the latest version's content
		latestVersion := fmt.Sprintf("%s/versions/latest", secret.Name)
		accessReq := &secretmanagerpb.AccessSecretVersionRequest{Name: latestVersion}
		result, err := client.AccessSecretVersion(ctx, accessReq)
		var payload string
		if err == nil && result != nil && result.Payload != nil {
			payload = string(result.Payload.Data)
		} else {
			payload = "(unable to fetch latest version or secret is empty)"
		}

		secrets = append(secrets, map[string]any{
			"name":    secret.Name,
			"payload": payload,
		})
	}

	// Build the secrets slice but do not include it in the output
	_ = secrets // keep for possible future use, but not returned

	output := map[string]any{
		// "projectId": input.ProjectID,
		"status": "Listed all secrets in the project",
	}
	out, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func parseAndValidateListGCPSecretParams(args map[string]any) (*ListGCPSecretInput, error) {
	input := &ListGCPSecretInput{}
	if v, ok := args["projectId"]; ok && v != nil {
		input.ProjectID = v.(string)
	}
	return input, nil
}
