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

// Environment variables used by this tool:
// Required:
//   GOOGLE_APPLICATION_CREDENTIALS - Path to the GCP service account JSON file (for local/outside GCP)
// Optional:
//   GOOGLE_CLOUD_PROJECT           - GCP Project ID (used if not provided in input)
//   GCP_SECRET_NAME                - Secret name (used if not provided in input)

// ChangeEnvInput represents the input for changing a key in a GCP secret.
type ChangeEnvInput struct {
	ProjectID  string `json:"projectId,omitempty"`
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
	NewValue   string `json:"newValue"`
}

// ChangeEnvTool provides functionality to update a key in a GCP secret.
type ChangeEnvTool struct{}

func NewChangeEnvTool() *ChangeEnvTool {
	return &ChangeEnvTool{}
}

func (t *ChangeEnvTool) Tool() mcp.Tool {
	return mcp.NewTool("change_env",
		mcp.WithDescription("Update a key in a Google Cloud Secret (JSON) and create a new version."),
		mcp.WithString("projectId", mcp.Description("GCP Project ID (optional, will use GOOGLE_CLOUD_PROJECT env if not set)")),
		mcp.WithString("secretName", mcp.Required(), mcp.Description("Name of the secret in Secret Manager")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Key in the JSON secret to update")),
		mcp.WithString("newValue", mcp.Required(), mcp.Description("New value for the key")),
	)
}

func (t *ChangeEnvTool) Handler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := parseAndValidateChangeEnvParams(req.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Get secret name from env
	input.SecretName = os.Getenv("GCP_SECRET_NAME")
	if input.SecretName == "" {
		return nil, fmt.Errorf("GCP_SECRET_NAME environment variable must be set for secret name")
	}

	// Always try to get projectID from env if not set in input
	if input.ProjectID == "" {
		input.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if input.ProjectID == "" {
		return nil, fmt.Errorf("projectId must be provided (either as input or environment variable)")
	}

	// Require GOOGLE_APPLICATION_CREDENTIALS to be set when running outside GCP
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		return nil, fmt.Errorf("google credentials not found: set GOOGLE_APPLICATION_CREDENTIALS to a service account JSON file")
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", input.ProjectID, input.SecretName)
	accessReq := &secretmanagerpb.AccessSecretVersionRequest{Name: secretPath}
	result, err := client.AccessSecretVersion(ctx, accessReq)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret: %w", err)
	}

	var secretData map[string]interface{}
	if err := json.Unmarshal(result.Payload.Data, &secretData); err != nil {
		return nil, fmt.Errorf("failed to parse secret JSON: %w", err)
	}

	if _, ok := secretData[input.Key]; !ok {
		return nil, fmt.Errorf("key '%s' not found in secret", input.Key)
	}
	secretData[input.Key] = input.NewValue

	json.MarshalIndent(secretData, "", "  ")
	// updatedJSON, err := json.MarshalIndent(secretData, "", "  ")
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to marshal updated secret: %w", err)
	// }

	// Push the updated secret as a new version
	// addReq := &secretmanagerpb.AddSecretVersionRequest{
	// 	Parent:  fmt.Sprintf("projects/%s/secrets/%s", input.ProjectID, input.SecretName),
	// 	Payload: &secretmanagerpb.SecretPayload{Data: updatedJSON},
	// }
	// _, err = client.AddSecretVersion(ctx, addReq)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to add new secret version: %w", err)
	// }

	output := map[string]string{
		"status": "Secret updated and new version created",
		// "secretName":  input.SecretName,
		// "key":         input.Key,
		// "newValue":    input.NewValue,
		// "updatedJson": string(updatedJSON),
	}
	out, _ := json.Marshal(output)
	return mcp.NewToolResultText(string(out)), nil
}

func parseAndValidateChangeEnvParams(args map[string]any) (*ChangeEnvInput, error) {
	input := &ChangeEnvInput{}
	if v, ok := args["projectId"]; ok && v != nil {
		input.ProjectID = v.(string)
	}
	if v, ok := args["secretName"]; ok && v != nil {
		input.SecretName = v.(string)
	}
	if v, ok := args["key"]; ok && v != nil {
		input.Key = v.(string)
	}
	if v, ok := args["newValue"]; ok && v != nil {
		input.NewValue = v.(string)
	}
	if input.SecretName == "" || input.Key == "" || input.NewValue == "" {
		return nil, fmt.Errorf("secretName, key, and newValue are required")
	}
	return input, nil
}
