package db

import (
	"path/filepath"
	"testing"
)

func TestPruneOrphanCustomModels(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDatabase(dbPath)
	if err != nil {
		t.Fatalf("OpenDatabase failed: %v", err)
	}
	defer db.Close()

	if err := EnsureCoreSchema(db); err != nil {
		t.Fatalf("EnsureCoreSchema failed: %v", err)
	}

	repo := NewRepo(db)

	// Create valid provider node
	validNodeID := "openai-compatible-chat-valid-1111-2222"
	_, err = repo.CreateProviderNode(validNodeID, "openai-compatible", "ValidNode", `{"prefix":"vn"}`)
	if err != nil {
		t.Fatalf("CreateProviderNode failed: %v", err)
	}

	// Insert customModels into kv
	_, err = db.Exec(`
		INSERT INTO kv (scope, key, value) VALUES
		('customModels', 'openai-compatible-chat-valid-1111-2222|model-1|llm', '{"providerAlias":"openai-compatible-chat-valid-1111-2222","id":"model-1"}'),
		('customModels', 'openai-compatible-chat-ghost-3333-4444|ghost-model|llm', '{"providerAlias":"openai-compatible-chat-ghost-3333-4444","id":"ghost-model"}'),
		('customModels', 'standard-provider|other-model|llm', '{"providerAlias":"standard-provider","id":"other-model"}')
	`)
	if err != nil {
		t.Fatalf("insert kv failed: %v", err)
	}

	deleted, err := PruneOrphanCustomModels(db)
	if err != nil {
		t.Fatalf("PruneOrphanCustomModels failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted orphan, got %d", deleted)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(1) FROM kv WHERE scope = 'customModels' AND key LIKE '%ghost%'`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected ghost custom model to be deleted, count = %d", count)
	}

	var validCount int
	_ = db.QueryRow(`SELECT COUNT(1) FROM kv WHERE scope = 'customModels' AND key LIKE '%valid%'`).Scan(&validCount)
	if validCount != 1 {
		t.Fatalf("expected valid custom model to survive, count = %d", validCount)
	}
}

