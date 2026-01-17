package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/quantumflow/quantumflow/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DgraphSemanticStore implements SemanticStore using Dgraph
type DgraphSemanticStore struct {
	client *dgo.Dgraph
	conn   *grpc.ClientConn
}

// NewDgraphSemanticStore creates a new Dgraph-backed semantic store
func NewDgraphSemanticStore(config *Config) (*DgraphSemanticStore, error) {
	// Connect to Dgraph gRPC endpoint
	conn, err := grpc.Dial(config.DgraphAlphaURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Dgraph: %w", err)
	}

	client := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	store := &DgraphSemanticStore{
		client: client,
		conn:   conn,
	}

	// Initialize schema
	if err := store.initSchema(context.Background()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema sets up the Dgraph schema for entities and relationships
func (s *DgraphSemanticStore) initSchema(ctx context.Context) error {
	schema := `
		type Entity {
			entity.id: string
			entity.name: string
			entity.type: string
			entity.attributes: string
			entity.created: datetime
			entity.updated: datetime
			relationships: [Relationship]
		}

		type Relationship {
			rel.id: string
			rel.type: string
			rel.confidence: float
			rel.created: datetime
			from: uid
			to: uid
		}

		entity.id: string @index(exact) @upsert .
		entity.name: string @index(fulltext, trigram) .
		entity.type: string @index(exact) .
		entity.attributes: string .
		entity.created: datetime @index(hour) .
		entity.updated: datetime .

		rel.id: string @index(exact) .
		rel.type: string @index(exact) .
		rel.confidence: float .
		rel.created: datetime .

		from: uid @reverse .
		to: uid @reverse .
		relationships: [uid] @reverse .
	`

	op := &api.Operation{Schema: schema}
	return s.client.Alter(ctx, op)
}

// StoreEntity stores an entity in the knowledge graph
func (s *DgraphSemanticStore) StoreEntity(ctx context.Context, entity *models.Entity) error {
	// Serialize attributes
	attributesJSON, err := json.Marshal(entity.Attributes)
	if err != nil {
		return fmt.Errorf("failed to marshal attributes: %w", err)
	}

	mutation := &api.Mutation{
		CommitNow: true,
		SetJson: []byte(fmt.Sprintf(`{
			"entity.id": "%s",
			"entity.name": "%s",
			"entity.type": "%s",
			"entity.attributes": %s,
			"entity.created": "%s",
			"entity.updated": "%s",
			"dgraph.type": "Entity"
		}`, entity.ID, entity.Name, entity.Type, attributesJSON,
			time.Now().Format(time.RFC3339),
			time.Now().Format(time.RFC3339))),
	}

	txn := s.client.NewTxn()
	defer txn.Discard(ctx)

	_, err = txn.Mutate(ctx, mutation)
	return err
}

// StoreRelationship adds a relationship between entities
func (s *DgraphSemanticStore) StoreRelationship(ctx context.Context, rel *models.Relationship) error {
	// First, find UIDs for from and to entities
	fromUID, err := s.getEntityUID(ctx, rel.FromID)
	if err != nil {
		return fmt.Errorf("failed to find from entity: %w", err)
	}

	toUID, err := s.getEntityUID(ctx, rel.ToID)
	if err != nil {
		return fmt.Errorf("failed to find to entity: %w", err)
	}

	mutation := &api.Mutation{
		CommitNow: true,
		SetJson: []byte(fmt.Sprintf(`{
			"uid": "_:rel",
			"rel.id": "%s",
			"rel.type": "%s",
			"rel.confidence": %f,
			"rel.created": "%s",
			"from": {"uid": "%s"},
			"to": {"uid": "%s"},
			"dgraph.type": "Relationship"
		}`, rel.ID, rel.Type, rel.Confidence, time.Now().Format(time.RFC3339),
			fromUID, toUID)),
	}

	txn := s.client.NewTxn()
	defer txn.Discard(ctx)

	_, err = txn.Mutate(ctx, mutation)
	return err
}

// QueryEntities finds entities matching criteria
func (s *DgraphSemanticStore) QueryEntities(ctx context.Context, query string) ([]*models.Entity, error) {
	// GraphQL-style query
	q := fmt.Sprintf(`{
		entities(func: alloftext(entity.name, "%s")) {
			uid
			entity.id
			entity.name
			entity.type
			entity.attributes
			entity.created
			entity.updated
		}
	}`, query)

	txn := s.client.NewReadOnlyTxn()
	defer txn.Discard(ctx)

	resp, err := txn.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	var result struct {
		Entities []struct {
			UID        string `json:"uid"`
			ID         string `json:"entity.id"`
			Name       string `json:"entity.name"`
			Type       string `json:"entity.type"`
			Attributes string `json:"entity.attributes"`
		} `json:"entities"`
	}

	if err := json.Unmarshal(resp.Json, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	entities := make([]*models.Entity, len(result.Entities))
	for i, e := range result.Entities {
		var attrs map[string]interface{}
		json.Unmarshal([]byte(e.Attributes), &attrs)

		entities[i] = &models.Entity{
			ID:         e.ID,
			Name:       e.Name,
			Type:       e.Type,
			Attributes: attrs,
		}
	}

	return entities, nil
}

// Traverse performs graph traversal from a starting entity
func (s *DgraphSemanticStore) Traverse(ctx context.Context, startID string, depth int) ([]*models.Entity, error) {
	// Build recursive query with specified depth
	q := fmt.Sprintf(`{
		traverse(func: eq(entity.id, "%s")) @recurse(depth: %d) {
			uid
			entity.id
			entity.name
			entity.type
			from
			to
		}
	}`, startID, depth)

	txn := s.client.NewReadOnlyTxn()
	defer txn.Discard(ctx)

	resp, err := txn.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("traverse failed: %w", err)
	}

	var result struct {
		Traverse []struct {
			ID   string `json:"entity.id"`
			Name string `json:"entity.name"`
			Type string `json:"entity.type"`
		} `json:"traverse"`
	}

	if err := json.Unmarshal(resp.Json, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	entities := make([]*models.Entity, len(result.Traverse))
	for i, e := range result.Traverse {
		entities[i] = &models.Entity{
			ID:   e.ID,
			Name: e.Name,
			Type: e.Type,
		}
	}

	return entities, nil
}

// ResolveEntity finds or merges duplicate entities
func (s *DgraphSemanticStore) ResolveEntity(ctx context.Context, name string, entityType string) (*models.Entity, error) {
	// Search for existing entity with similar name and type
	q := fmt.Sprintf(`{
		entities(func: alloftext(entity.name, "%s")) @filter(eq(entity.type, "%s")) {
			uid
			entity.id
			entity.name
			entity.type
		}
	}`, name, entityType)

	txn := s.client.NewReadOnlyTxn()
	defer txn.Discard(ctx)

	resp, err := txn.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("resolve failed: %w", err)
	}

	var result struct {
		Entities []struct {
			ID   string `json:"entity.id"`
			Name string `json:"entity.name"`
			Type string `json:"entity.type"`
		} `json:"entities"`
	}

	if err := json.Unmarshal(resp.Json, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Entities) > 0 {
		// Return first matching entity
		return &models.Entity{
			ID:   result.Entities[0].ID,
			Name: result.Entities[0].Name,
			Type: result.Entities[0].Type,
		}, nil
	}

	return nil, nil // No match found
}

// getEntityUID retrieves the Dgraph UID for an entity by its ID
func (s *DgraphSemanticStore) getEntityUID(ctx context.Context, entityID string) (string, error) {
	q := fmt.Sprintf(`{
		entity(func: eq(entity.id, "%s")) {
			uid
		}
	}`, entityID)

	txn := s.client.NewReadOnlyTxn()
	defer txn.Discard(ctx)

	resp, err := txn.Query(ctx, q)
	if err != nil {
		return "", err
	}

	var result struct {
		Entity []struct {
			UID string `json:"uid"`
		} `json:"entity"`
	}

	if err := json.Unmarshal(resp.Json, &result); err != nil {
		return "", err
	}

	if len(result.Entity) == 0 {
		return "", fmt.Errorf("entity not found: %s", entityID)
	}

	return result.Entity[0].UID, nil
}

// Close closes the Dgraph connection
func (s *DgraphSemanticStore) Close() error {
	return s.conn.Close()
}
