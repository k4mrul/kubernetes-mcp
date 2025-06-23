package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/k4mrul/kubernetes-mcp/src/validation"
	"github.com/mark3labs/mcp-go/mcp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ListResourcesInput represents the input parameters for listing Kubernetes resources.
type ListResourcesInput struct {
	Kind           string `json:"kind"`
	GroupFilter    string `json:"groupFilter,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	LabelSelector  string `json:"labelSelector,omitempty"`
	FieldSelector  string `json:"fieldSelector,omitempty"`
	Limit          int64  `json:"limit,omitempty"`
	TimeoutSeconds int64  `json:"timeoutSeconds,omitempty"`
	ShowDetails    bool   `json:"showDetails,omitempty"`
}

// ResourceWithStatus represents a resource with its status information extracted.
type ResourceWithStatus struct {
	Name      string      `json:"name"`
	Namespace string      `json:"namespace,omitempty"`
	Kind      string      `json:"kind"`
	Status    interface{} `json:"status,omitempty"`
}

// PodSummary represents a minimal summary for a Pod
// Only used for kind == "Pod"
type PodSummary struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Phase        string `json:"phase"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restartCount"`
	StartTime    string `json:"startTime"`
}

// DeploymentSummary represents a minimal summary for a Deployment
// Only used for kind == "Deployment"
type DeploymentSummary struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Replicas    int32  `json:"replicas"`
	Available   int32  `json:"available"`
	Unavailable int32  `json:"unavailable"`
	Updated     int32  `json:"updated"`
	Ready       int32  `json:"ready"`
}

// ServiceSummary represents a minimal summary for a Service
// Only used for kind == "Service"
type ServiceSummary struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Type      string   `json:"type"`
	ClusterIP string   `json:"clusterIP"`
	Ports     []string `json:"ports"`
}

// IngressSummary represents a minimal summary for an Ingress
// Only used for kind == "Ingress"
type IngressSummary struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Hosts     []string `json:"hosts"`
	Addresses []string `json:"addresses"`
}

// ListTool provides functionality to list Kubernetes resources by kind.
type ListTool struct {
	client Client
}

// NewListTool creates a new ListTool instance with the provided Kubernetes client.
func NewListTool(client Client) ListTool {
	return ListTool{client: client}
}

// Tool returns the MCP tool definition for listing Kubernetes resources.
func (l ListTool) Tool() mcp.Tool {
	return mcp.NewTool("list_resources",
		mcp.WithDescription("List Kubernetes resources with their status information by default, with advanced filtering options"),
		mcp.WithString("kind",
			mcp.Description("Kind of the Kubernetes resource, e.g., Pod, Deployment, Service, ConfigMap, or any CRD. Use 'all' with groupFilter to discover all resource types for a project."),
		),
		mcp.WithString("groupFilter",
			mcp.Description("Filter by API group substring to discover all resources from a project (e.g., 'flux' for FluxCD, 'argo' for ArgoCD, 'istio' for Istio). When used with kind='all', returns all matching resource types."),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace to list resources from (leave empty for all namespaces, use 'default' for default namespace)"),
		),
		mcp.WithString("labelSelector",
			mcp.Description("Filter resources by label selector (e.g., 'app=nginx', 'tier=frontend,environment!=prod')"),
		),
		mcp.WithString("fieldSelector",
			mcp.Description("Filter resources by field selector (e.g., 'metadata.name=my-pod', 'spec.nodeName=node1')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of resources to return (useful for large clusters, default: no limit)"),
		),
		mcp.WithNumber("timeoutSeconds",
			mcp.Description("Timeout for the list operation in seconds (default: 30)"),
		),
		mcp.WithBoolean("showDetails",
			mcp.Description("Return complete resource objects instead of just name and status (default: false)"),
		),
	)
}

// Handler processes requests to list Kubernetes resources by kind and namespace.
func (l ListTool) Handler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	input, err := parseAndValidateListParams(req.Params.Arguments)
	if err != nil {
		return nil, err
	}

	// Handle groupFilter functionality for discovering resources
	if input.GroupFilter != "" {
		if input.Kind == "all" || input.Kind == "" {
			// Discovery mode: return all resource types for the group
			return l.handleGroupDiscovery(input.GroupFilter)
		} else {
			// Filter mode: find specific kind within the group
			return l.handleGroupFilteredList(ctx, input)
		}
	}

	// Original functionality for specific kind
	gvrMatch, err := l.discoverResourceByKind(input.Kind)
	if err != nil {
		return nil, err
	}

	if input.ShowDetails {
		// Return full resource details (complete objects)
		resources, err := l.listResourceDetails(ctx, gvrMatch, input)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(resources)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource details: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	} else {
		// Default: Return resources with status information
		resourcesWithStatus, err := l.listResourcesWithStatus(ctx, gvrMatch, input)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(resourcesWithStatus)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resources with status: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// handleGroupDiscovery returns all available resource types for a given group filter
func (l ListTool) handleGroupDiscovery(groupFilter string) (*mcp.CallToolResult, error) {
	discoClient, err := l.client.DiscoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiResourceLists, err := discoClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover resources: %w", err)
	}

	matches, err := findGVRsByGroupSubstring(apiResourceLists, groupFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to find resources by group substring: %w", err)
	}

	if len(matches) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf(`{"message": "No resources found for group filter '%s'", "availableResources": []}`, groupFilter)), nil
	}

	// Format the discovered resource types
	discoveredTypes := make([]map[string]interface{}, 0)
	for _, match := range matches {
		discoveredTypes = append(discoveredTypes, map[string]interface{}{
			"kind":       match.apiRes.Kind,
			"group":      match.groupVersion,
			"resource":   match.apiRes.Name,
			"namespaced": match.namespaced,
			"shortNames": match.apiRes.ShortNames,
		})
	}

	result := map[string]interface{}{
		"groupFilter":     groupFilter,
		"discoveredTypes": discoveredTypes,
		"totalFound":      len(matches),
		"message":         fmt.Sprintf("Found %d resource types matching group filter '%s'", len(matches), groupFilter),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal discovery result: %w", err)
	}
	return mcp.NewToolResultText(string(out)), nil
}

// handleGroupFilteredList lists resources of a specific kind within a filtered group
func (l ListTool) handleGroupFilteredList(ctx context.Context, input *ListResourcesInput) (*mcp.CallToolResult, error) {
	discoClient, err := l.client.DiscoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiResourceLists, err := discoClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover resources: %w", err)
	}

	// First find all resources in the group
	matches, err := findGVRsByGroupSubstring(apiResourceLists, input.GroupFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to find resources by group substring: %w", err)
	}

	// Find the specific kind within the group
	var gvrMatch *gvrMatch
	kindLower := strings.ToLower(input.Kind)
	for _, match := range matches {
		if strings.ToLower(match.apiRes.Kind) == kindLower || strings.ToLower(match.apiRes.Name) == kindLower {
			gvrMatch = match
			break
		}
		// Check short names too
		for _, shortName := range match.apiRes.ShortNames {
			if strings.ToLower(shortName) == kindLower {
				gvrMatch = match
				break
			}
		}
		if gvrMatch != nil {
			break
		}
	}

	if gvrMatch == nil {
		return nil, fmt.Errorf("kind '%s' not found in group filter '%s'", input.Kind, input.GroupFilter)
	}

	// Now list the resources using the found GVR
	if input.ShowDetails {
		resources, err := l.listResourceDetails(ctx, gvrMatch, input)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(resources)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource details: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	} else {
		resourcesWithStatus, err := l.listResourcesWithStatus(ctx, gvrMatch, input)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(resourcesWithStatus)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resources with status: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// discoverResourceByKind discovers and returns the GroupVersionResource match for a given kind.
func (l ListTool) discoverResourceByKind(kind string) (*gvrMatch, error) {
	discoClient, err := l.client.DiscoClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiResourceLists, err := discoClient.ServerPreferredResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover resources: %w", err)
	}

	return findGVRByKind(apiResourceLists, kind)
}

// listResourceDetails retrieves full details of all resources matching the given GVR and input parameters.
func (l ListTool) listResourceDetails(ctx context.Context, gvrMatch *gvrMatch, input *ListResourcesInput) (interface{}, error) {
	ri, err := l.client.ResourceInterface(*gvrMatch.ToGroupVersionResource(), gvrMatch.namespaced, input.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource interface: %w", err)
	}

	listOptions := l.buildListOptions(input)
	unstructList, err := ri.List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return unstructList, nil
}

// buildListOptions creates metav1.ListOptions from the input parameters.
func (l ListTool) buildListOptions(input *ListResourcesInput) metav1.ListOptions {
	listOptions := metav1.ListOptions{
		LabelSelector: input.LabelSelector,
		FieldSelector: input.FieldSelector,
	}

	if input.Limit > 0 {
		listOptions.Limit = input.Limit
	}

	if input.TimeoutSeconds > 0 {
		listOptions.TimeoutSeconds = &input.TimeoutSeconds
	} else {
		// Default timeout of 30 seconds
		defaultTimeout := int64(30)
		listOptions.TimeoutSeconds = &defaultTimeout
	}

	return listOptions
}

// listResourcesWithStatus retrieves resources and extracts their status information.
func (l ListTool) listResourcesWithStatus(ctx context.Context, gvrMatch *gvrMatch, input *ListResourcesInput) ([]interface{}, error) {
	ri, err := l.client.ResourceInterface(*gvrMatch.ToGroupVersionResource(), gvrMatch.namespaced, input.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource interface: %w", err)
	}

	listOptions := l.buildListOptions(input)
	unstructList, err := ri.List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	var result []interface{}
	kind := strings.ToLower(gvrMatch.apiRes.Kind)
	for _, item := range unstructList.Items {
		switch kind {
		case "pod":
			pod := PodSummary{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			}
			status, found, _ := unstructured.NestedMap(item.Object, "status")
			if found {
				if phase, ok := status["phase"].(string); ok {
					pod.Phase = phase
				}
				if startTime, ok := status["startTime"].(string); ok {
					pod.StartTime = startTime
				}
				if csArr, ok := status["containerStatuses"].([]interface{}); ok {
					allReady := true
					restartCount := 0
					for _, cs := range csArr {
						if csMap, ok := cs.(map[string]interface{}); ok {
							if ready, ok := csMap["ready"].(bool); ok {
								allReady = allReady && ready
							}
							if rc, ok := csMap["restartCount"].(float64); ok {
								restartCount += int(rc)
							}
						}
					}
					pod.Ready = allReady
					pod.RestartCount = restartCount
				}
			}
			result = append(result, pod)
		case "deployment":
			dep := DeploymentSummary{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			}
			status, found, _ := unstructured.NestedMap(item.Object, "status")
			if found {
				if v, ok := status["replicas"].(float64); ok {
					dep.Replicas = int32(v)
				}
				if v, ok := status["availableReplicas"].(float64); ok {
					dep.Available = int32(v)
				}
				if v, ok := status["unavailableReplicas"].(float64); ok {
					dep.Unavailable = int32(v)
				}
				if v, ok := status["updatedReplicas"].(float64); ok {
					dep.Updated = int32(v)
				}
				if v, ok := status["readyReplicas"].(float64); ok {
					dep.Ready = int32(v)
				}
			}
			result = append(result, dep)
		case "service":
			svc := ServiceSummary{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			}
			spec, found, _ := unstructured.NestedMap(item.Object, "spec")
			if found {
				if t, ok := spec["type"].(string); ok {
					svc.Type = t
				}
				if cip, ok := spec["clusterIP"].(string); ok {
					svc.ClusterIP = cip
				}
				if ports, ok := spec["ports"].([]interface{}); ok {
					var portList []string
					for _, p := range ports {
						if portMap, ok := p.(map[string]interface{}); ok {
							portStr := ""
							if name, ok := portMap["name"].(string); ok {
								portStr += name + ":"
							}
							if port, ok := portMap["port"].(float64); ok {
								portStr += fmt.Sprintf("%d", int(port))
							}
							if protocol, ok := portMap["protocol"].(string); ok {
								portStr += "/" + protocol
							}
							portList = append(portList, portStr)
						}
					}
					svc.Ports = portList
				}
			}
			result = append(result, svc)
		case "ingress":
			ing := IngressSummary{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			}
			status, found, _ := unstructured.NestedMap(item.Object, "status")
			if found {
				if lb, ok := status["loadBalancer"].(map[string]interface{}); ok {
					if ingressArr, ok := lb["ingress"].([]interface{}); ok {
						var addresses []string
						for _, addr := range ingressArr {
							if addrMap, ok := addr.(map[string]interface{}); ok {
								if ip, ok := addrMap["ip"].(string); ok {
									addresses = append(addresses, ip)
								}
								if hostname, ok := addrMap["hostname"].(string); ok {
									addresses = append(addresses, hostname)
								}
							}
						}
						ing.Addresses = addresses
					}
				}
			}
			spec, found, _ := unstructured.NestedMap(item.Object, "spec")
			if found {
				if rules, ok := spec["rules"].([]interface{}); ok {
					var hosts []string
					for _, rule := range rules {
						if ruleMap, ok := rule.(map[string]interface{}); ok {
							if host, ok := ruleMap["host"].(string); ok {
								hosts = append(hosts, host)
							}
						}
					}
					ing.Hosts = hosts
				}
			}
			result = append(result, ing)
		default:
			resourceWithStatus := l.extractResourceStatus(&item)
			result = append(result, resourceWithStatus)
		}
	}
	return result, nil
}

// extractResourceStatus extracts the status section from a resource.
func (l ListTool) extractResourceStatus(obj *unstructured.Unstructured) ResourceWithStatus {
	resource := ResourceWithStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Kind:      obj.GetKind(),
	}

	// Extract the entire status section
	if status, found, err := unstructured.NestedMap(obj.Object, "status"); found && err == nil {
		resource.Status = status
	}

	return resource
}

// parseAndValidateListParams validates and extracts parameters from request arguments.
func parseAndValidateListParams(args map[string]any) (*ListResourcesInput, error) {
	input := &ListResourcesInput{}

	// Optional: groupFilter
	if groupFilter, ok := args["groupFilter"].(string); ok {
		input.GroupFilter = groupFilter
	}

	// Kind: Required unless groupFilter is used for discovery
	if kindVal, ok := args["kind"].(string); ok && kindVal != "" {
		input.Kind = kindVal
		if err := validation.ValidateKind(input.Kind); err != nil {
			return nil, fmt.Errorf("invalid kind: %w", err)
		}
	} else if input.GroupFilter == "" {
		return nil, errors.New("kind must be provided when groupFilter is not specified")
	}

	// Optional: namespace
	if ns, ok := args["namespace"].(string); ok {
		input.Namespace = ns
		if err := validation.ValidateNamespace(input.Namespace); err != nil {
			return nil, fmt.Errorf("invalid namespace: %w", err)
		}
	}
	if input.Namespace == "" {
		input.Namespace = metav1.NamespaceAll
	}

	// Optional: labelSelector
	if labelSelector, ok := args["labelSelector"].(string); ok {
		input.LabelSelector = labelSelector
		if err := validation.ValidateLabelSelector(input.LabelSelector); err != nil {
			return nil, fmt.Errorf("invalid labelSelector: %w", err)
		}
	}

	// Optional: fieldSelector
	if fieldSelector, ok := args["fieldSelector"].(string); ok {
		input.FieldSelector = fieldSelector
	}

	// Optional: limit
	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		input.Limit = int64(limit)
	}

	// Optional: timeoutSeconds
	if timeoutSeconds, ok := args["timeoutSeconds"].(float64); ok && timeoutSeconds > 0 {
		input.TimeoutSeconds = int64(timeoutSeconds)
	}

	// Optional: showDetails
	if showDetails, ok := args["showDetails"].(bool); ok {
		input.ShowDetails = showDetails
	}

	return input, nil
}

// gvrMatchList is a collection of GroupVersionResource matches.
type gvrMatchList []*gvrMatch

// ToGroupVersionResources converts the match list to a slice of GroupVersionResource pointers.
func (f *gvrMatchList) ToGroupVersionResources() []*schema.GroupVersionResource {
	var gvrList []*schema.GroupVersionResource
	for _, found := range *f {
		gvr := found.ToGroupVersionResource()
		if gvr == nil {
			continue
		}
		gvrList = append(gvrList, gvr)
	}
	return gvrList
}

// gvrMatch represents a matched Kubernetes API resource with its group/version and namespacing info.
type gvrMatch struct {
	apiRes       *metav1.APIResource
	groupVersion string
	namespaced   bool
}

// newGvrMatch creates a new gvrMatch instance.
func newGvrMatch(apiRes *metav1.APIResource, groupVersion string, namespaced bool) *gvrMatch {
	return &gvrMatch{
		apiRes,
		groupVersion,
		namespaced,
	}
}

// ToGroupVersionResource converts the match to a GroupVersionResource. Returns nil if invalid.
func (f *gvrMatch) ToGroupVersionResource() *schema.GroupVersionResource {
	if f.groupVersion == "" {
		return nil
	}
	if f.apiRes == nil {
		return nil
	}
	parts := strings.Split(f.groupVersion, "/")
	var group, version string
	if len(parts) == 1 {
		group = ""
		version = parts[0]
	} else {
		group = parts[0]
		version = parts[1]
	}
	gvr := &schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: f.apiRes.Name,
	}
	return gvr
}

// findGVRsByGroupSubstring finds all resources whose group contains the specified substring (case-insensitive).
func findGVRsByGroupSubstring(apiResourceLists []*metav1.APIResourceList, groupSubstring string) (gvrMatchList, error) {
	target := strings.ToLower(groupSubstring)
	var matches gvrMatchList
	for _, apiResList := range apiResourceLists {
		if apiResList == nil {
			continue
		}
		gv := apiResList.GroupVersion
		if !strings.Contains(gv, target) {
			continue
		}
		for _, r := range apiResList.APIResources {
			matches = append(matches, newGvrMatch(&r, gv, r.Namespaced))
		}
	}

	return matches, nil
}

// findGVRByKind finds a resource by matching against plural name, Kind, or short names (case-insensitive).
func findGVRByKind(apiResourceLists []*metav1.APIResourceList, kind string) (*gvrMatch, error) {
	target := strings.ToLower(kind)
	var found *gvrMatch

	for _, apiResList := range apiResourceLists {
		if apiResList == nil {
			continue
		}
		gv := apiResList.GroupVersion

		for _, r := range apiResList.APIResources {
			nameLower := strings.ToLower(r.Name)
			kindLower := strings.ToLower(r.Kind)

			if nameLower == target || kindLower == target {
				found = newGvrMatch(&r, gv, r.Namespaced)
				break
			}

			for _, sn := range r.ShortNames {
				if strings.ToLower(sn) == target {
					found = newGvrMatch(&r, gv, r.Namespaced)
					break
				}
			}
			if found != nil {
				break
			}
		}

		if found != nil {
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("cannot find resource '%s'", kind)
	}
	if found.ToGroupVersionResource() == nil {
		return nil, fmt.Errorf("cannot find resource '%s'", kind)
	}
	return found, nil
}
