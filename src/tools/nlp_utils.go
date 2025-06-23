package tools

import (
	"context"
	"encoding/json"
	"fmt"

	langchainopenai "github.com/tmc/langchaingo/llms/openai"
	langchainprompt "github.com/tmc/langchaingo/prompts"
)

// ExtractChangeEnvEntities uses langchaingo to extract key, value, and service from a user request.
func ExtractChangeEnvEntities(ctx context.Context, userRequest string) (key, value, service string, err error) {
	llm, err := langchainopenai.New()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to initialize langchaingo: %w", err)
	}

	promptText := `Extract the key, value, and service from the following request.
If the service is not mentioned, return 'NONE' for service.
Request: {{.request}}
Return as JSON: {"key":..., "value":..., "service":...}`

	prompt := langchainprompt.NewPromptTemplate(promptText, []string{"request"})

	input := map[string]any{"request": userRequest}
	promptStr, err := prompt.Format(input)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to format prompt: %w", err)
	}

	resp, err := llm.Call(ctx, promptStr, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("langchaingo LLM call failed: %w", err)
	}

	// Parse the JSON response
	var result struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Service string `json:"service"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return "", "", "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result.Key, result.Value, result.Service, nil
}

// TransformStatusQuery analyzes user input like "what is the status for content service, is it running?"
// and generates a structured instruction for the backend using langchaingo prompt templates.
func TransformStatusQuery(ctx context.Context, userRequest string) (serviceName string, instruction string, err error) {
	llm, err := langchainopenai.New()
	if err != nil {
		return "", "", fmt.Errorf("failed to initialize langchaingo: %w", err)
	}

	promptText := `Given the following user request, extract the service or pod name and generate a step-by-step instruction to:
1. Call list_resources for pods in all namespaces.
2. Find the pod(s) whose name contains the service name.
3. Check if the pod(s) are running (status.phase == 'Running').

User request: {{.request}}
Return as JSON: {"service":..., "instruction":...}`

	prompt := langchainprompt.NewPromptTemplate(promptText, []string{"request"})
	input := map[string]any{"request": userRequest}
	promptStr, err := prompt.Format(input)
	if err != nil {
		return "", "", fmt.Errorf("failed to format prompt: %w", err)
	}

	resp, err := llm.Call(ctx, promptStr, nil)
	if err != nil {
		return "", "", fmt.Errorf("langchaingo LLM call failed: %w", err)
	}

	var result struct {
		Service     string `json:"service"`
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return "", "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result.Service, result.Instruction, nil
}
