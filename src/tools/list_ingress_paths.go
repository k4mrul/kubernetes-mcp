package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ListIngressPathsInput represents the input parameters for listing ingress paths.
type ListIngressPathsInput struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// IngressPath represents a path configuration from an ingress.
type IngressPath struct {
	Path        string `json:"path"`
	PathType    string `json:"pathType,omitempty"`
	ServiceName string `json:"serviceName"`
}

// IngressPathsResponse represents the response containing all paths from an ingress.
type IngressPathsResponse struct {
	IngressName string        `json:"ingressName"`
	Namespace   string        `json:"namespace"`
	Paths       []IngressPath `json:"paths"`
}

// ListIngressPathsTool provides functionality to list paths from a specific ingress.
type ListIngressPathsTool struct {
	client Client
}

// NewListIngressPathsTool creates a new ListIngressPathsTool instance with the provided Kubernetes client.
func NewListIngressPathsTool(client Client) *ListIngressPathsTool {
	return &ListIngressPathsTool{client: client}
}

// Tool returns the MCP tool definition for listing ingress paths.
func (l *ListIngressPathsTool) Tool() mcp.Tool {
	return mcp.NewTool("list_ingress_paths",
		mcp.WithDescription("List all paths from a specific Kubernetes ingress resource in staging namespace"),
		mcp.WithString("unused",
			mcp.Description("This parameter is unused but required for schema validation"),
		),
	)
}

// Handler processes requests to list paths from a specific ingress.
func (l *ListIngressPathsTool) Handler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input := &ListIngressPathsInput{}

	// Get ingress name from environment variable, fallback to hardcoded value
	ingressName := os.Getenv("INGRESS_NAME")
	if ingressName == "" {
		ingressName = "test-arafat-im"
	}
	input.Name = ingressName

	// Always use staging namespace
	input.Namespace = "staging"

	// Get the ingress resource
	ingress, err := l.getIngress(ctx, input)
	if err != nil {
		return nil, err
	}

	// Extract paths from the ingress
	response, err := l.extractIngressPaths(ingress, input)
	if err != nil {
		return nil, err
	}

	// Marshal the response
	out, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ingress paths: %w", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

// getIngress retrieves the ingress resource from the cluster.
func (l *ListIngressPathsTool) getIngress(ctx context.Context, input *ListIngressPathsInput) (*unstructured.Unstructured, error) {
	// Discover the ingress resource GVR
	gvrMatch, err := l.discoverIngressResource()
	if err != nil {
		return nil, err
	}

	// Get resource interface
	ri, err := l.client.ResourceInterface(*gvrMatch.ToGroupVersionResource(), gvrMatch.namespaced, input.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource interface: %w", err)
	}

	// Get the ingress resource
	ingress, err := ri.Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress %s in namespace %s: %w", input.Name, input.Namespace, err)
	}

	return ingress, nil
}

// discoverIngressResource discovers the ingress resource GVR.
func (l *ListIngressPathsTool) discoverIngressResource() (*gvrMatch, error) {
	discoClient, err := l.client.DiscoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiResourceLists, err := discoClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover resources: %w", err)
	}

	return findGVRByKind(apiResourceLists, "Ingress")
}

// extractIngressPaths extracts all paths from the ingress resource.
func (l *ListIngressPathsTool) extractIngressPaths(ingress *unstructured.Unstructured, input *ListIngressPathsInput) (*IngressPathsResponse, error) {
	response := &IngressPathsResponse{
		IngressName: input.Name,
		Namespace:   input.Namespace,
		Paths:       []IngressPath{},
	}

	// Extract spec from the ingress
	spec, found, err := unstructured.NestedMap(ingress.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress spec: %w", err)
	}
	if !found {
		return response, nil
	}

	// Extract rules and paths
	rules, found, err := unstructured.NestedSlice(spec, "rules")
	if err != nil {
		return nil, fmt.Errorf("failed to get ingress rules: %w", err)
	}
	if !found {
		return response, nil
	}

	// Process each rule
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		// Get HTTP paths
		httpPaths, found, err := unstructured.NestedSlice(ruleMap, "http", "paths")
		if err != nil || !found {
			continue
		}

		// Process each path
		for _, pathEntry := range httpPaths {
			pathMap, ok := pathEntry.(map[string]interface{})
			if !ok {
				continue
			}

			ingressPath := IngressPath{}

			// Extract path
			if path, found, _ := unstructured.NestedString(pathMap, "path"); found {
				ingressPath.Path = path
			}

			// Extract pathType
			if pathType, found, _ := unstructured.NestedString(pathMap, "pathType"); found {
				ingressPath.PathType = pathType
			}

			// Extract backend service information
			if backend, found, _ := unstructured.NestedMap(pathMap, "backend"); found {
				// Try service backend first (newer API)
				if service, found, _ := unstructured.NestedMap(backend, "service"); found {
					if serviceName, found, _ := unstructured.NestedString(service, "name"); found {
						ingressPath.ServiceName = serviceName
					}
				} else {
					// Fallback to legacy backend format
					if serviceName, found, _ := unstructured.NestedString(backend, "serviceName"); found {
						ingressPath.ServiceName = serviceName
					}
				}
			}

			response.Paths = append(response.Paths, ingressPath)
		}
	}

	return response, nil
}
