package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unsafe"

	"github.com/go-redis/redis/v8"
	"github.com/quantumflow/quantumflow/internal/models"
)

// RedisEpisodicStore implements EpisodicStore using Redis with vector indexing
type RedisEpisodicStore struct {
	client    *redis.Client
	indexName string
	ttl       time.Duration
}

// NewRedisEpisodicStore creates a new Redis-backed episodic memory store
func NewRedisEpisodicStore(config *Config) (*RedisEpisodicStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisURL,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	store := &RedisEpisodicStore{
		client:    client,
		indexName: "memory:episodic:idx",
		ttl:       time.Duration(config.RetentionDays) * 24 * time.Hour,
	}

	// Create vector index if it doesn't exist
	if err := store.createIndex(ctx, config.EmbeddingDimensions); err != nil {
		return nil, fmt.Errorf("failed to create vector index: %w", err)
	}

	return store, nil
}

// createIndex creates a Redis vector search index
func (s *RedisEpisodicStore) createIndex(ctx context.Context, dimensions int) error {
	// Check if index already exists
	_, err := s.client.Do(ctx, "FT.INFO", s.indexName).Result()
	if err == nil {
		return nil // Index already exists
	}

	// Create index with vector similarity search
	// FT.CREATE index ON HASH PREFIX 1 memory:episodic: SCHEMA
	//   content TEXT
	//   embedding VECTOR FLAT 6 DIM <dimensions> DISTANCE_METRIC COSINE TYPE FLOAT32
	//   timestamp NUMERIC SORTABLE
	args := []interface{}{
		"FT.CREATE", s.indexName,
		"ON", "HASH",
		"PREFIX", "1", "memory:episodic:",
		"SCHEMA",
		"content", "TEXT",
		"embedding", "VECTOR", "FLAT", "6",
		"DIM", dimensions,
		"DISTANCE_METRIC", "COSINE",
		"TYPE", "FLOAT32",
		"timestamp", "NUMERIC", "SORTABLE",
		"type", "TAG",
	}

	if err := s.client.Do(ctx, args...).Err(); err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	return nil
}

// Store stores a memory entry with vector embedding
func (s *RedisEpisodicStore) Store(ctx context.Context, memory *models.Memory) error {
	if memory.ID == "" {
		memory.ID = fmt.Sprintf("memory:episodic:%d", time.Now().UnixNano())
	}

	// Serialize embedding as byte array
	embeddingBytes, err := serializeEmbedding(memory.Embedding)
	if err != nil {
		return fmt.Errorf("failed to serialize embedding: %w", err)
	}

	// Serialize metadata
	metadataJSON, err := json.Marshal(memory.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Store in Redis hash
	pipe := s.client.Pipeline()
	pipe.HSet(ctx, memory.ID, map[string]interface{}{
		"content":   memory.Content,
		"embedding": embeddingBytes,
		"timestamp": memory.Timestamp.Unix(),
		"type":      string(memory.Type),
		"score":     memory.Score,
		"metadata":  metadataJSON,
	})

	// Set TTL if configured
	if s.ttl > 0 {
		pipe.Expire(ctx, memory.ID, s.ttl)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	return nil
}

// Search performs vector similarity search
func (s *RedisEpisodicStore) Search(ctx context.Context, embedding []float32, k int) ([]*models.Memory, error) {
	embeddingBytes, err := serializeEmbedding(embedding)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize query embedding: %w", err)
	}

	// FT.SEARCH index "*=>[KNN k @embedding $query_vec]" PARAMS 2 query_vec <embedding> DIALECT 2
	args := []interface{}{
		"FT.SEARCH", s.indexName,
		fmt.Sprintf("*=>[KNN %d @embedding $query_vec]", k),
		"PARAMS", "2", "query_vec", embeddingBytes,
		"DIALECT", "2",
		"RETURN", "6", "content", "timestamp", "type", "score", "metadata", "__embedding_score",
		"LIMIT", "0", k,
	}

	result, err := s.client.Do(ctx, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	// Parse results
	memories, err := s.parseSearchResults(result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return memories, nil
}

// parseSearchResults parses Redis FT.SEARCH results into Memory objects
func (s *RedisEpisodicStore) parseSearchResults(result interface{}) ([]*models.Memory, error) {
	// Redis returns: [totalResults, [id1, [field1, value1, field2, value2, ...]], [id2, ...]]
	results, ok := result.([]interface{})
	if !ok || len(results) == 0 {
		return []*models.Memory{}, nil
	}

	// Skip first element (total count)
	if len(results) < 2 {
		return []*models.Memory{}, nil
	}

	var memories []*models.Memory
	for i := 1; i < len(results); i++ {
		doc, ok := results[i].([]interface{})
		if !ok || len(doc) < 2 {
			continue
		}

		id := fmt.Sprint(doc[0])
		fields, ok := doc[1].([]interface{})
		if !ok {
			continue
		}

		memory := &models.Memory{ID: id}
		for j := 0; j < len(fields); j += 2 {
			if j+1 >= len(fields) {
				break
			}

			field := fmt.Sprint(fields[j])
			value := fmt.Sprint(fields[j+1])

			switch field {
			case "content":
				memory.Content = value
			case "type":
				memory.Type = models.MemoryType(value)
			case "score", "__embedding_score":
				fmt.Sscanf(value, "%f", &memory.Score)
			case "timestamp":
				var ts int64
				fmt.Sscanf(value, "%d", &ts)
				memory.Timestamp = time.Unix(ts, 0)
			case "metadata":
				json.Unmarshal([]byte(value), &memory.Metadata)
			}
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

// Delete removes a memory entry
func (s *RedisEpisodicStore) Delete(ctx context.Context, id string) error {
	return s.client.Del(ctx, id).Err()
}

// Count returns total number of episodic memories
func (s *RedisEpisodicStore) Count(ctx context.Context) (int64, error) {
	// Count all keys matching the prefix
	iter := s.client.Scan(ctx, 0, "memory:episodic:*", 0).Iterator()
	count := int64(0)
	for iter.Next(ctx) {
		count++
	}
	if err := iter.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// Close closes the Redis connection
func (s *RedisEpisodicStore) Close() error {
	return s.client.Close()
}

// serializeEmbedding converts float32 slice to byte array for Redis
func serializeEmbedding(embedding []float32) ([]byte, error) {
	if embedding == nil {
		return nil, fmt.Errorf("embedding is nil")
	}

	// Convert float32 to bytes (Redis expects raw bytes for vector fields)
	bytes := make([]byte, len(embedding)*4)
	for i, val := range embedding {
		bits := *(*uint32)(unsafe.Pointer(&val))
		bytes[i*4] = byte(bits)
		bytes[i*4+1] = byte(bits >> 8)
		bytes[i*4+2] = byte(bits >> 16)
		bytes[i*4+3] = byte(bits >> 24)
	}

	return bytes, nil
}
