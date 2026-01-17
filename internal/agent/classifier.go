package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantumflow/quantumflow/internal/models"
)

// RuleBasedClassifier implements query classification using keyword matching
type RuleBasedClassifier struct {
	rules map[models.AgentType][]string
}

// NewRuleBasedClassifier creates a new rule-based classifier
func NewRuleBasedClassifier() *RuleBasedClassifier {
	return &RuleBasedClassifier{
		rules: map[models.AgentType][]string{
			models.AgentTypeCode: {
				"code", "function", "class", "debug", "refactor", "bug",
				"implement", "parse", "ast", "syntax", "compile", "test",
				"method", "variable", "import", "package", "module",
				"golang", "python", "javascript", "typescript", "java",
				"error", "exception", "stacktrace", "lint",
			},
			models.AgentTypeData: {
				"data", "database", "sql", "query", "table", "schema",
				"analytics", "pandas", "dataframe", "csv", "json",
				"aggregate", "group", "join", "select", "insert",
				"update", "delete", "migration", "index", "postgres",
				"mysql", "mongodb", "redis", "statistics", "chart",
			},
			models.AgentTypeInfra: {
				"deploy", "infrastructure", "server", "container", "docker",
				"kubernetes", "k8s", "terraform", "ansible", "aws",
				"gcp", "azure", "cloud", "scaling", "load balancer",
				"nginx", "service", "pod", "node", "cluster", "helm",
				"vpc", "network", "firewall", "instance", "vm",
			},
			models.AgentTypeSec: {
				"security", "vulnerability", "cve", "owasp", "xss",
				"sql injection", "csrf", "authentication", "authorization",
				"encryption", "decrypt", "certificate", "ssl", "tls",
				"firewall", "audit", "compliance", "pen test", "scan",
				"malware", "threat", "attack", "breach", "exploit",
			},
		},
	}
}

// Classify returns the best agent type for a query
func (c *RuleBasedClassifier) Classify(ctx context.Context, query string) (models.AgentType, float64, error) {
	classifications, err := c.ClassifyMulti(ctx, query, 1)
	if err != nil {
		return "", 0, err
	}

	if len(classifications) == 0 {
		// Default to code agent if no match
		return models.AgentTypeCode, 0.3, nil
	}

	return classifications[0].AgentType, classifications[0].Confidence, nil
}

// ClassifyMulti returns top-k agent types with confidence scores
func (c *RuleBasedClassifier) ClassifyMulti(ctx context.Context, query string, k int) ([]Classification, error) {
	query = strings.ToLower(query)
	words := strings.Fields(query)

	// Calculate scores for each agent type
	scores := make(map[models.AgentType]float64)
	matches := make(map[models.AgentType][]string)

	for agentType, keywords := range c.rules {
		matchCount := 0
		matchedKeywords := []string{}

		for _, keyword := range keywords {
			for _, word := range words {
				if strings.Contains(word, keyword) || strings.Contains(keyword, word) {
					matchCount++
					matchedKeywords = append(matchedKeywords, keyword)
					break
				}
			}
		}

		if matchCount > 0 {
			// Score = matches / total words, with bonus for multiple matches
			score := float64(matchCount) / float64(len(words))
			score = score + (float64(matchCount) * 0.1) // Bonus for multiple matches
			if score > 1.0 {
				score = 1.0
			}

			scores[agentType] = score
			matches[agentType] = matchedKeywords
		}
	}

	// Convert to classifications and sort by score
	var classifications []Classification
	for agentType, score := range scores {
		classifications = append(classifications, Classification{
			AgentType:  agentType,
			Confidence: score,
			Reasoning:  fmt.Sprintf("Matched keywords: %s", strings.Join(matches[agentType], ", ")),
		})
	}

	// Sort by confidence (descending)
	for i := 0; i < len(classifications); i++ {
		for j := i + 1; j < len(classifications); j++ {
			if classifications[j].Confidence > classifications[i].Confidence {
				classifications[i], classifications[j] = classifications[j], classifications[i]
			}
		}
	}

	// Return top-k
	if len(classifications) > k {
		classifications = classifications[:k]
	}

	return classifications, nil
}

// AddRule adds a new keyword rule for an agent type
func (c *RuleBasedClassifier) AddRule(agentType models.AgentType, keywords []string) {
	if c.rules[agentType] == nil {
		c.rules[agentType] = []string{}
	}
	c.rules[agentType] = append(c.rules[agentType], keywords...)
}
