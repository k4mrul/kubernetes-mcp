package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

// RolloutRestartInput represents the input for restarting a deployment.
type RolloutRestartInput struct {
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
}

// RolloutTool provides functionality to rollout/restart deployments.
type RolloutTool struct {
	client Client
}

// NewRolloutTool creates a new RolloutTool with the provided Kubernetes client.
func NewRolloutTool(client Client) *RolloutTool {
	return &RolloutTool{client: client}
}

// Tool returns the MCP tool definition for rollout restart.
func (r *RolloutTool) Tool() mcp.Tool {
	return mcp.NewTool("rollout_restart",
		mcp.WithDescription("Perform a rolling restart of a Kubernetes deployment (like 'kubectl rollout restart deployment')"),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace of the deployment (defaults to 'default' if not specified)"),
		),
		mcp.WithString("deployment",
			mcp.Required(),
			mcp.Description("Name of the deployment to restart"),
		),
	)
}

// Handler performs the rollout restart.
func (r *RolloutTool) Handler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := parseAndValidateRolloutParams(req.Params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and validate rollout params: %w", err)
	}

	clientset, err := r.client.Clientset()
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset: %w", err)
	}

	deploymentsClient := clientset.AppsV1().Deployments(input.Namespace)
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().Format(time.RFC3339)))
	_, err = deploymentsClient.Patch(ctx, input.Deployment, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to patch deployment: %w", err)
	}

	result := map[string]string{
		"status":     "Deployment restarted",
		"deployment": input.Deployment,
		"namespace":  input.Namespace,
	}
	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

// parseAndValidateRolloutParams validates and parses the input parameters.
func parseAndValidateRolloutParams(args map[string]any) (*RolloutRestartInput, error) {
	input := &RolloutRestartInput{}

	if ns, ok := args["namespace"]; ok && ns != nil {
		input.Namespace = ns.(string)
	} else {
		input.Namespace = metav1.NamespaceDefault
	}

	if dep, ok := args["deployment"]; ok && dep != nil {
		input.Deployment = dep.(string)
	}

	if input.Deployment == "" {
		return nil, fmt.Errorf("deployment must be provided")
	}

	return input, nil
}
