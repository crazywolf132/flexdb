package flexdb

import (
	"encoding/json"
	"os"
	"testing"
)

// TestEntity is a sample entity for testing purposes
type TestEntity struct {
	ID    string
	Name  string
	Value int
}

func (te *TestEntity) GetID() string   { return te.ID }
func (te *TestEntity) SetID(id string) { te.ID = id }

func TestNewDatabase(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	// Create a valid initial database state
	initialData := map[string]map[string]interface{}{
		"test": {
			"1": map[string]interface{}{
				"ID":    "1",
				"Name":  "Test Entity",
				"Value": 42,
			},
		},
	}
	initialJSON, _ := json.Marshal(initialData)
	if err := os.WriteFile(dbPath, initialJSON, 0644); err != nil {
		t.Fatalf("Failed to create initial database file: %v", err)
	}

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create new database: %v", err)
	}

	if db == nil {
		t.Fatal("NewDatabase returned nil")
	}

	// Verify that the data was loaded correctly
	tx := db.Transact(true)
	defer tx.Rollback()

	entity, ok := tx.Get("test", "1")
	if !ok {
		t.Fatal("Failed to retrieve test entity")
	}

	genericEntity, ok := entity.(*GenericEntity)
	if !ok {
		t.Fatal("Retrieved entity is not a GenericEntity")
	}

	if genericEntity.Fields["Name"] != "Test Entity" {
		t.Errorf("Unexpected entity name: got %v, want %v", genericEntity.Fields["Name"], "Test Entity")
	}
}

func TestDatabaseTransactions(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	// Test read-only transaction
	readTx := db.Transact(true)
	if !readTx.readOnly {
		t.Error("Expected read-only transaction")
	}
	readTx.Rollback()

	// Test write transaction
	writeTx := db.Transact(false)
	if writeTx.readOnly {
		t.Error("Expected write transaction")
	}

	entity := &TestEntity{ID: "1", Name: "Test", Value: 42}
	err := writeTx.Set("test", entity)
	if err != nil {
		t.Fatalf("Failed to set entity: %v", err)
	}

	err = writeTx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify the entity was saved
	readTx = db.Transact(true)
	defer readTx.Rollback()

	savedEntity, ok := readTx.Get("test", "1")
	if !ok {
		t.Fatal("Failed to retrieve saved entity")
	}

	if savedEntity.(*TestEntity).Name != "Test" {
		t.Error("Retrieved entity does not match saved entity")
	}
}

func TestQuerying(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	// Add some test data
	writeTx := db.Transact(false)
	writeTx.Set("test", &TestEntity{ID: "1", Name: "Alice", Value: 30})
	writeTx.Set("test", &TestEntity{ID: "2", Name: "Bob", Value: 25})
	writeTx.Set("test", &TestEntity{ID: "3", Name: "Charlie", Value: 35})
	writeTx.Commit()

	// Test querying
	readTx := db.Transact(true)
	defer readTx.Rollback()

	query := readTx.NewQuery("test").Where("Value", 25)
	results, err := query.Execute()
	if err != nil {
		t.Fatalf("Query execution failed: %v", err)
	}

	if len(results) != 1 || results[0].(*TestEntity).Name != "Bob" {
		t.Error("Query returned unexpected results")
	}

	// Test WhereLike
	query = readTx.NewQuery("test").WhereLike("Name", "li")
	results, err = query.Execute()
	if err != nil {
		t.Fatalf("WhereLike query execution failed: %v", err)
	}

	if len(results) != 2 {
		t.Error("WhereLike query returned unexpected number of results")
	}
}

func TestIndexing(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	// Add an index
	db.AddIndex("test", "Name")

	// Add some test data
	writeTx := db.Transact(false)
	writeTx.Set("test", &TestEntity{ID: "1", Name: "Alice", Value: 30})
	writeTx.Set("test", &TestEntity{ID: "2", Name: "Bob", Value: 25})
	writeTx.Commit()

	// Test querying with index
	readTx := db.Transact(true)
	defer readTx.Rollback()

	query := readTx.NewQuery("test").Where("Name", "Alice")
	results, err := query.Execute()
	if err != nil {
		t.Fatalf("Query execution failed: %v", err)
	}

	if len(results) != 1 || results[0].(*TestEntity).Name != "Alice" {
		t.Error("Query with index returned unexpected results")
	}
}

func TestHooks(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	hookCalled := false
	db.RegisterHook("pre-set", func(tx *Transaction, entityType string, entity Entity) error {
		hookCalled = true
		return nil
	})

	writeTx := db.Transact(false)
	writeTx.Set("test", &TestEntity{ID: "1", Name: "Test", Value: 42})
	writeTx.Commit()

	if !hookCalled {
		t.Error("Pre-set hook was not called")
	}
}

func TestMigrations(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create new database: %v", err)
	}

	db.AddMigration(1, func(tx *Transaction) error {
		return tx.Set("test", &TestEntity{ID: "migration1", Name: "Migration 1", Value: 1})
	}, func(tx *Transaction) error {
		return tx.Delete("test", "migration1")
	})

	err = db.Migrate(1)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Use a new transaction to check the result
	readTx := db.Transact(true)
	defer readTx.Rollback()

	entity, ok := readTx.Get("test", "migration1")
	if !ok {
		t.Fatal("Migration entity not found")
	}

	testEntity, ok := entity.(*TestEntity)
	if !ok {
		t.Fatalf("Retrieved entity is not a TestEntity: %T", entity)
	}

	if testEntity.Name != "Migration 1" {
		t.Errorf("Migration entity has unexpected data: got %s, want Migration 1", testEntity.Name)
	}
}

func TestBatchOperations(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	writeTx := db.Transact(false)
	defer writeTx.Rollback()

	entities := []Entity{
		&TestEntity{ID: "1", Name: "Entity 1", Value: 10},
		&TestEntity{ID: "2", Name: "Entity 2", Value: 20},
		&TestEntity{ID: "3", Name: "Entity 3", Value: 30},
	}

	err := writeTx.BatchSet("test", entities)
	if err != nil {
		t.Fatalf("BatchSet failed: %v", err)
	}

	writeTx.Commit()

	// Verify batch set
	readTx := db.Transact(true)
	defer readTx.Rollback()

	for _, e := range entities {
		_, ok := readTx.Get("test", e.GetID())
		if !ok {
			t.Errorf("Entity %s not found after BatchSet", e.GetID())
		}
	}

	// Test BatchDelete
	writeTx = db.Transact(false)
	defer writeTx.Rollback()

	err = writeTx.BatchDelete("test", []string{"1", "3"})
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	writeTx.Commit()

	// Verify batch delete
	readTx = db.Transact(true)
	defer readTx.Rollback()

	_, ok1 := readTx.Get("test", "1")
	_, ok3 := readTx.Get("test", "3")
	if ok1 || ok3 {
		t.Error("BatchDelete did not remove entities as expected")
	}

	_, ok2 := readTx.Get("test", "2")
	if !ok2 {
		t.Error("BatchDelete removed an entity it shouldn't have")
	}
}

func TestCaching(t *testing.T) {
	dbPath := "./test_db.json"
	defer os.Remove(dbPath)

	db, _ := NewDatabase(dbPath)

	// Write initial data
	writeTx := db.Transact(false)
	writeTx.Set("test", &TestEntity{ID: "1", Name: "Initial Entity", Value: 100})
	err := writeTx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit initial transaction: %v", err)
	}

	// Read the entity to cache it
	readTx := db.Transact(true)
	initialEntity, ok := readTx.Get("test", "1")
	if !ok || initialEntity.(*TestEntity).Name != "Initial Entity" {
		t.Fatalf("Failed to read initial entity")
	}
	readTx.Rollback()

	// Modify the entity in a new transaction, but don't commit
	modifyTx := db.Transact(false)
	modifyTx.Set("test", &TestEntity{ID: "1", Name: "Modified Entity", Value: 200})

	// Read in a separate transaction, should still see the initial entity
	readTx2 := db.Transact(true)
	cachedEntity, _ := readTx2.Get("test", "1")
	if cachedEntity.(*TestEntity).Name != "Initial Entity" {
		t.Errorf("Cache is not working as expected. Got %s, want Initial Entity", cachedEntity.(*TestEntity).Name)
	}
	readTx2.Rollback()

	// Commit the modification
	err = modifyTx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit modification: %v", err)
	}

	// Read again, should now see the modified entity
	readTx3 := db.Transact(true)
	updatedEntity, _ := readTx3.Get("test", "1")
	if updatedEntity.(*TestEntity).Name != "Modified Entity" {
		t.Errorf("Cache did not update after commit. Got %s, want Modified Entity", updatedEntity.(*TestEntity).Name)
	}
	readTx3.Rollback()
}
