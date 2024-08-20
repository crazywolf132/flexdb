package flexdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

type GenericEntity struct {
	ID     string
	Fields map[string]interface{}
}

func (ge *GenericEntity) GetID() string   { return ge.ID }
func (ge *GenericEntity) SetID(id string) { ge.ID = id }

// Entity represents a generic database entity
type Entity interface {
	GetID() string
	SetID(string)
}

// Database represents the main database object
type Database struct {
	path       string
	mu         sync.RWMutex
	data       map[string]map[string]Entity
	indexes    map[string]map[string]map[string][]string
	hooks      map[string][]Hook
	cache      *cache.Cache
	migrations []Migration
}

// Hook is a function that can be registered to run before or after certain database operations
type Hook func(tx *Transaction, entityType string, entity Entity) error

// Migration represents a database migration
type Migration struct {
	Version int
	Up      func(*Transaction) error
	Down    func(*Transaction) error
}

// NewDatabase creates and initializes a new database
func NewDatabase(path string) (*Database, error) {
	db := &Database{
		path:       path,
		data:       make(map[string]map[string]Entity),
		indexes:    make(map[string]map[string]map[string][]string),
		hooks:      make(map[string][]Hook),
		cache:      cache.New(5*time.Minute, 10*time.Minute),
		migrations: []Migration{},
	}

	if err := db.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return db, nil
}

func (db *Database) load() error {
	data, err := os.ReadFile(db.path)
	if err != nil {
		return err
	}

	var rawData map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawData); err != nil {
		return err
	}

	for entityType, entities := range rawData {
		db.data[entityType] = make(map[string]Entity)
		for id, rawEntity := range entities {
			var entity map[string]interface{}
			if err := json.Unmarshal(rawEntity, &entity); err != nil {
				return err
			}
			db.data[entityType][id] = &GenericEntity{
				ID:     id,
				Fields: entity,
			}
		}
	}

	return nil
}

func (db *Database) save() error {
	data, err := json.MarshalIndent(db.data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(db.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(db.path, data, 0644)
}

// AddIndex creates an index for faster querying
func (db *Database) AddIndex(entityType, field string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.indexes[entityType] == nil {
		db.indexes[entityType] = make(map[string]map[string][]string)
	}
	db.indexes[entityType][field] = make(map[string][]string)

	for id, entity := range db.data[entityType] {
		value := reflect.ValueOf(entity).Elem().FieldByName(field).String()
		db.indexes[entityType][field][value] = append(db.indexes[entityType][field][value], id)
	}
}

// RegisterHook adds a hook to be executed before or after certain operations
func (db *Database) RegisterHook(operation string, hook Hook) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.hooks[operation] = append(db.hooks[operation], hook)
}

// AddMigration adds a new migration to the database
func (db *Database) AddMigration(version int, up, down func(*Transaction) error) {
	db.migrations = append(db.migrations, Migration{
		Version: version,
		Up:      up,
		Down:    down,
	})
}

// Migrate runs all pending migrations up to the specified version
func (db *Database) Migrate(targetVersion int) error {
	tx := db.Transact(false)
	defer tx.Rollback() // This will handle unlocking properly

	currentVersion, err := getCurrentVersion(tx)
	if err != nil {
		return err
	}

	for _, migration := range db.migrations {
		if migration.Version > currentVersion && migration.Version <= targetVersion {
			if err := migration.Up(tx); err != nil {
				return err
			}
			if err := setCurrentVersion(tx, migration.Version); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// Transaction represents a database transaction
type Transaction struct {
	db        *Database
	readOnly  bool
	changes   map[string]map[string]Entity
	committed bool
}

// Transact starts a new transaction
func (db *Database) Transact(readOnly bool) *Transaction {
	return &Transaction{
		db:        db,
		readOnly:  readOnly,
		changes:   make(map[string]map[string]Entity),
		committed: false,
	}
}

// Commit applies the transaction changes and releases the lock
func (tx *Transaction) Commit() error {
	if tx.readOnly {
		return nil
	}

	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()

	for entityType, entities := range tx.changes {
		if tx.db.data[entityType] == nil {
			tx.db.data[entityType] = make(map[string]Entity)
		}
		for id, entity := range entities {
			if entity == nil {
				delete(tx.db.data[entityType], id)
				tx.db.cache.Delete(getCacheKey(entityType, id))
			} else {
				tx.db.data[entityType][id] = entity
				tx.db.cache.Set(getCacheKey(entityType, id), entity, cache.DefaultExpiration)
			}
			// Update indexes
			for field, index := range tx.db.indexes[entityType] {
				value := reflect.ValueOf(entity).Elem().FieldByName(field).String()
				index[value] = append(index[value], id)
			}
		}
	}

	tx.committed = true
	return tx.db.save()
}

// Rollback discards the transaction changes
func (tx *Transaction) Rollback() {
	// No need to unlock anything, as we're using deferred unlocks in the methods that acquire locks
	tx.changes = make(map[string]map[string]Entity)
}

// Get retrieves an entity by type and ID
func (tx *Transaction) Get(entityType string, id string) (Entity, bool) {
	// Check the transaction's changes first
	if changedEntities, ok := tx.changes[entityType]; ok {
		if entity, ok := changedEntities[id]; ok {
			return entity, true
		}
	}

	// If not in transaction changes, check the database (which includes committed cache)
	tx.db.mu.RLock()
	defer tx.db.mu.RUnlock()

	if cachedEntity, found := tx.db.cache.Get(getCacheKey(entityType, id)); found {
		return cachedEntity.(Entity), true
	}

	if entities, ok := tx.db.data[entityType]; ok {
		if entity, ok := entities[id]; ok {
			// Cache the entity for future use
			tx.db.cache.Set(getCacheKey(entityType, id), entity, cache.DefaultExpiration)
			return entity, true
		}
	}

	return nil, false
}

// GetAll retrieves all entities of a given type
func (tx *Transaction) GetAll(entityType string) []Entity {
	var entities []Entity
	if entityMap, ok := tx.db.data[entityType]; ok {
		for _, entity := range entityMap {
			entities = append(entities, entity)
		}
	}
	if changedEntities, ok := tx.changes[entityType]; ok {
		for id, entity := range changedEntities {
			if entity == nil {
				// Remove deleted entities
				for i, e := range entities {
					if e.GetID() == id {
						entities = append(entities[:i], entities[i+1:]...)
						break
					}
				}
			} else {
				// Update or add changed entities
				found := false
				for i, e := range entities {
					if e.GetID() == id {
						entities[i] = entity
						found = true
						break
					}
				}
				if !found {
					entities = append(entities, entity)
				}
			}
		}
	}
	return entities
}

// Set adds or updates an entity
func (tx *Transaction) Set(entityType string, entity Entity) error {
	if tx.readOnly {
		return fmt.Errorf("cannot modify data in a read-only transaction")
	}

	// Run pre-set hooks
	for _, hook := range tx.db.hooks["pre-set"] {
		if err := hook(tx, entityType, entity); err != nil {
			return err
		}
	}

	if tx.changes[entityType] == nil {
		tx.changes[entityType] = make(map[string]Entity)
	}
	tx.changes[entityType][entity.GetID()] = entity

	// Run post-set hooks
	for _, hook := range tx.db.hooks["post-set"] {
		if err := hook(tx, entityType, entity); err != nil {
			return err
		}
	}

	return nil
}

// Delete removes an entity
func (tx *Transaction) Delete(entityType string, id string) error {
	if tx.readOnly {
		return fmt.Errorf("cannot modify data in a read-only transaction")
	}

	// Run pre-delete hooks
	entity, exists := tx.Get(entityType, id)
	if exists {
		for _, hook := range tx.db.hooks["pre-delete"] {
			if err := hook(tx, entityType, entity); err != nil {
				return err
			}
		}
	}

	if tx.changes[entityType] == nil {
		tx.changes[entityType] = make(map[string]Entity)
	}
	tx.changes[entityType][id] = nil

	// Run post-delete hooks
	if exists {
		for _, hook := range tx.db.hooks["post-delete"] {
			if err := hook(tx, entityType, entity); err != nil {
				return err
			}
		}
	}

	return nil
}

// BatchSet adds or updates multiple entities in a single operation
func (tx *Transaction) BatchSet(entityType string, entities []Entity) error {
	for _, entity := range entities {
		if err := tx.Set(entityType, entity); err != nil {
			return err
		}
	}
	return nil
}

// BatchDelete removes multiple entities in a single operation
func (tx *Transaction) BatchDelete(entityType string, ids []string) error {
	for _, id := range ids {
		if err := tx.Delete(entityType, id); err != nil {
			return err
		}
	}
	return nil
}

// Query represents a database query
type Query struct {
	tx         *Transaction
	entityType string
	filters    []func(Entity) bool
	limit      int
	offset     int
	orderBy    string
	orderDesc  bool
}

// Where adds a filter to the query
func (q *Query) Where(field string, value interface{}) *Query {
	q.filters = append(q.filters, func(e Entity) bool {
		return reflect.ValueOf(e).Elem().FieldByName(field).Interface() == value
	})
	return q
}

// WhereIn adds a filter that checks if a field's value is in a given slice
func (q *Query) WhereIn(field string, values []interface{}) *Query {
	q.filters = append(q.filters, func(e Entity) bool {
		fieldValue := reflect.ValueOf(e).Elem().FieldByName(field).Interface()
		for _, v := range values {
			if fieldValue == v {
				return true
			}
		}
		return false
	})
	return q
}

// WhereLike adds a filter that checks if a field's value contains a given string
func (q *Query) WhereLike(field string, value string) *Query {
	q.filters = append(q.filters, func(e Entity) bool {
		fieldValue := reflect.ValueOf(e).Elem().FieldByName(field).String()
		return strings.Contains(fieldValue, value)
	})
	return q
}

// Limit sets the maximum number of results to return
func (q *Query) Limit(limit int) *Query {
	q.limit = limit
	return q
}

// Offset sets the number of results to skip
func (q *Query) Offset(offset int) *Query {
	q.offset = offset
	return q
}

// OrderBy sets the field to order results by
func (q *Query) OrderBy(field string, desc bool) *Query {
	q.orderBy = field
	q.orderDesc = desc
	return q
}

// Execute runs the query and returns the results
func (q *Query) Execute() ([]Entity, error) {
	entities := q.tx.GetAll(q.entityType)
	var results []Entity

	for _, entity := range entities {
		match := true
		for _, filter := range q.filters {
			if !filter(entity) {
				match = false
				break
			}
		}
		if match {
			results = append(results, entity)
		}
	}

	if q.orderBy != "" {
		sort.Slice(results, func(i, j int) bool {
			vi := reflect.ValueOf(results[i]).Elem().FieldByName(q.orderBy)
			vj := reflect.ValueOf(results[j]).Elem().FieldByName(q.orderBy)
			if q.orderDesc {
				return vi.Interface().(Comparable).Compare(vj.Interface().(Comparable)) > 0
			}
			return vi.Interface().(Comparable).Compare(vj.Interface().(Comparable)) < 0
		})
	}

	if q.offset > 0 {
		if q.offset >= len(results) {
			return []Entity{}, nil
		}
		results = results[q.offset:]
	}

	if q.limit > 0 && q.limit < len(results) {
		results = results[:q.limit]
	}

	return results, nil
}

// NewQuery creates a new query for the given entity type
func (tx *Transaction) NewQuery(entityType string) *Query {
	return &Query{
		tx:         tx,
		entityType: entityType,
	}
}

// Comparable is an interface for types that can be compared
type Comparable interface {
	Compare(Comparable) int
}

// Helper functions

func getCacheKey(entityType, id string) string {
	return fmt.Sprintf("%s:%s", entityType, id)
}

func getCurrentVersion(tx *Transaction) (int, error) {
	entity, ok := tx.Get("migration", "current_version")
	if !ok {
		return 0, nil
	}
	version, ok := entity.(*MigrationVersion)
	if !ok {
		return 0, fmt.Errorf("invalid entity type for migration version")
	}
	return version.Version, nil
}

func setCurrentVersion(tx *Transaction, version int) error {
	return tx.Set("migration", &MigrationVersion{ID: "current_version", Version: version})
}

// MigrationVersion represents the current migration version
type MigrationVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

func (mv *MigrationVersion) GetID() string   { return mv.ID }
func (mv *MigrationVersion) SetID(id string) { mv.ID = id }
