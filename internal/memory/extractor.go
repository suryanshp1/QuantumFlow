package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quantumflow/quantumflow/internal/inference"
	"github.com/quantumflow/quantumflow/internal/models"
)

// QwenExtractor implements Extractor using Qwen for extraction tasks
type QwenExtractor struct {
	client *inference.Client
}

// NewQwenExtractor creates a new Qwen-based extractor
func NewQwenExtractor(client *inference.Client) *QwenExtractor {
	return &QwenExtractor{client: client}
}

// ExtractFacts extracts factual statements from text
func (e *QwenExtractor) ExtractFacts(ctx context.Context, text string) ([]Fact, error) {
	prompt := fmt.Sprintf(`Extract all factual statements from the following text. Return as JSON array:
[{"statement": "...", "subject": "...", "predicate": "...", "object": "...", "confidence": 0.9}]

Text:
%s

JSON:`, text)

	infResult, err := e.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Parse JSON response
	response := cleanJSONResponse(infResult.Response)
	var facts []Fact
	if err := json.Unmarshal([]byte(response), &facts); err != nil {
		return nil, fmt.Errorf("failed to parse facts: %w", err)
	}

	// Add metadata
	now := time.Now()
	for i := range facts {
		facts[i].ID = fmt.Sprintf("fact:%d:%d", now.Unix(), i)
		facts[i].Timestamp = now
		facts[i].Source = "qwen-extraction"
	}

	return facts, nil
}

// ExtractEntities extracts named entities from text
func (e *QwenExtractor) ExtractEntities(ctx context.Context, text string) ([]*models.Entity, error) {
	prompt := fmt.Sprintf(`Extract all named entities from the text. Return as JSON:
[{"name": "...", "type": "PERSON|ORGANIZATION|LOCATION|DATE|OTHER"}]

Text:
%s

JSON:`, text)

	result, err := e.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	response := cleanJSONResponse(result.Response)

	var entities []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.Unmarshal([]byte(response), &entities); err != nil {
		return nil, fmt.Errorf("failed to parse entities: %w", err)
	}

	// Convert to Entity models
	entityModels := make([]*models.Entity, len(entities))
	for i, e := range entities {
		entityModels[i] = &models.Entity{
			ID:         fmt.Sprintf("entity:%d:%d", time.Now().Unix(), i),
			Name:       e.Name,
			Type:       e.Type,
			Attributes: make(map[string]interface{}),
		}
	}

	return entityModels, nil
}

// ExtractRelationships identifies relationships between entities
func (e *QwenExtractor) ExtractRelationships(ctx context.Context, text string) ([]*models.Relationship, error) {
	prompt := fmt.Sprintf(`Extract relationships between entities. Return as JSON:
[{"from": "entity1", "to": "entity2", "type": "relationship_type", "confidence": 0.9}]

Text:
%s

JSON:`, text)

	result, err := e.client.GenerateSync(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	response := cleanJSONResponse(result.Response)

	var rels []struct {
		From       string  `json:"from"`
		To         string  `json:"to"`
		Type       string  `json:"type"`
		Confidence float64 `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(response), &rels); err != nil {
		return nil, fmt.Errorf("failed to parse relationships: %w", err)
	}

	// Convert to Relationship models
	relModels := make([]*models.Relationship, len(rels))
	for i, r := range rels {
		relModels[i] = &models.Relationship{
			ID:         fmt.Sprintf("rel:%d:%d", time.Now().Unix(), i),
			FromID:     r.From,
			ToID:       r.To,
			Type:       r.Type,
			Confidence: r.Confidence,
		}
	}

	return relModels, nil
}

// Summarize creates a concise summary of text
func (e *QwenExtractor) Summarize(ctx context.Context, text string, maxTokens int) (string, error) {
	prompt := fmt.Sprintf(`Summarize the following text in maximum %d tokens. Focus on key points:

%s

Summary:`, maxTokens, text)

	infResult, err := e.client.GenerateSync(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	return  strings.TrimSpace(infResult.Response), nil
}

// cleanJSONResponse extracts JSON from potentially markdown-wrapped response
func cleanJSONResponse(response string) string {
	// Remove markdown code blocks if present
	response = strings.TrimSpace(response)

	// Remove ```json and ``` markers
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}

	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}

	return strings.TrimSpace(response)
}
