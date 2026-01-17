package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/quantumflow/quantumflow/internal/models"
)

// BadgerProceduralStore implements ProceduralStore using BadgerDB
type BadgerProceduralStore struct {
	db *badger.DB
}

// NewBadgerProceduralStore creates a new BadgerDB-backed procedural store
func NewBadgerProceduralStore(config *Config) (*BadgerProceduralStore, error) {
	// Expand tilde in path
	path := expandPath(config.BadgerPath)

	opts := badger.DefaultOptions(path).
		WithLoggingLevel(badger.WARNING)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	return &BadgerProceduralStore{db: db}, nil
}

// StorePattern saves a workflow pattern
func (s *BadgerProceduralStore) StorePattern(ctx context.Context, pattern *models.WorkflowPattern) error {
	if pattern.ID == "" {
		pattern.ID = fmt.Sprintf("pattern:%d", time.Now().UnixNano())
	}

	data, err := json.Marshal(pattern)
	if err != nil {
		return fmt.Errorf("failed to marshal pattern: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("workflow:pattern:%s", pattern.ID))
		return txn.Set(key, data)
	})
}

// GetPattern retrieves a pattern by ID
func (s *BadgerProceduralStore) GetPattern(ctx context.Context, id string) (*models.WorkflowPattern, error) {
	var pattern models.WorkflowPattern

	err := s.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("workflow:pattern:%s", id))
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &pattern)
		})
	})

	if err == badger.ErrKeyNotFound {
		return nil, fmt.Errorf("pattern not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	return &pattern, nil
}

// FindSimilarPatterns finds patterns similar to given steps
func (s *BadgerProceduralStore) FindSimilarPatterns(ctx context.Context, steps []models.WorkflowStep, k int) ([]*models.WorkflowPattern, error) {
	// Extract action signatures for matching
	signatures := make([]string, len(steps))
	for i, step := range steps {
		signatures[i] = fmt.Sprintf("%s:%s", step.Action, step.Tool)
	}

	var patterns []*models.WorkflowPattern
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("workflow:pattern:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var pattern models.WorkflowPattern
				if err := json.Unmarshal(val, &pattern); err != nil {
					return nil // Skip malformed entries
				}

				// Calculate similarity score
				score := calculatePatternSimilarity(signatures, pattern.Steps)
				if score > 0.5 { // Threshold for similarity
					patterns = append(patterns, &pattern)
				}

				return nil
			})
			if err != nil {
				continue
			}

			// Limit results
			if len(patterns) >= k {
				break
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return patterns, nil
}

// UpdateFrequency increments pattern usage frequency
func (s *BadgerProceduralStore) UpdateFrequency(ctx context.Context, id string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("workflow:pattern:%s", id))

		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		var pattern models.WorkflowPattern
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &pattern)
		})
		if err != nil {
			return err
		}

		// Increment frequency and update last used
		pattern.Frequency++
		pattern.LastUsed = time.Now()

		data, err := json.Marshal(pattern)
		if err != nil {
			return err
		}

		return txn.Set(key, data)
	})
}

// GetTopPatterns returns most frequently used patterns
func (s *BadgerProceduralStore) GetTopPatterns(ctx context.Context, limit int) ([]*models.WorkflowPattern, error) {
	var patterns []*models.WorkflowPattern

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("workflow:pattern:")

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var pattern models.WorkflowPattern
				if err := json.Unmarshal(val, &pattern); err != nil {
					return nil
				}
				patterns = append(patterns, &pattern)
				return nil
			})
			if err != nil {
				continue
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by frequency (descending)
	sortByFrequency(patterns)

	// Limit results
	if len(patterns) > limit {
		patterns = patterns[:limit]
	}

	return patterns, nil
}

// Close closes the BadgerDB instance
func (s *BadgerProceduralStore) Close() error {
	return s.db.Close()
}

// calculatePatternSimilarity computes similarity between step signatures
func calculatePatternSimilarity(signatures []string, steps []models.WorkflowStep) float64 {
	if len(steps) == 0 {
		return 0
	}

	matches := 0
	for i, sig := range signatures {
		if i >= len(steps) {
			break
		}
		stepSig := fmt.Sprintf("%s:%s", steps[i].Action, steps[i].Tool)
		if sig == stepSig {
			matches++
		}
	}

	return float64(matches) / float64(max(len(signatures), len(steps)))
}

// sortByFrequency sorts patterns by frequency in descending order
func sortByFrequency(patterns []*models.WorkflowPattern) {
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			if patterns[j].Frequency > patterns[i].Frequency {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
