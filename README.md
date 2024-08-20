# 🚀 FlexDB: The Database That Bends Over Backwards for You

Welcome to FlexDB, the Go database library that's more flexible than a gymnast! Whether you're building a simple todo app or the next big thing in tech, FlexDB is here to make your data dance to your tune.

## 🌟 Features

- 🔍 Powerful querying capabilities
- 🏎️ Lightning-fast with built-in caching (reflects only committed changes)
- 🧘‍♀️ Flexible schema design
- 🎣 Hooks for custom logic
- 🏗️ Built-in migration system
- 📊 Indexing for speedy lookups
- 🦾 Support for batch operations

## 🚀 Quick Start

First, let's get this party started:

```go
import "github.com/foxycorps/flexdb"

db, err := flexdb.NewDatabase("./my_awesome_db.json")
if err != nil {
    panic("Oh no! The database doesn't want to play 😢")
}
```

## 🍼 Simple Use Cases

### The "Hello, World!" of Databases

Let's create a simple todo item:

```go
type Todo struct {
    ID    string `json:"id"`
    Task  string `json:"task"`
    Done  bool   `json:"done"`
}

func (t *Todo) GetID() string { return t.ID }
func (t *Todo) SetID(id string) { t.ID = id }

tx := db.Transact(false)
defer func() {
    if !tx.committed {
        tx.Rollback()
    }
}()

todo := &Todo{ID: "1", Task: "Learn FlexDB", Done: false}
err := tx.Set("todo", todo)
if err != nil {
    fmt.Println("Oops! Something went wrong:", err)
    return
}

err = tx.Commit()
if err != nil {
    fmt.Println("Commit failed! Time to panic:", err)
    return
}

fmt.Println("Todo created! Time to procrastinate...")
```

### Fetch All Todos

Feeling overwhelmed? Let's see all our todos:

```go
tx := db.Transact(true)
defer tx.Rollback()

todos := tx.GetAll("todo")
for _, todo := range todos {
    fmt.Printf("%s: %v\n", todo.(*Todo).Task, todo.(*Todo).Done)
}
```

## 🏋️ Medium Use Cases

### Query with Filters

Let's find all the todos that contain "FlexDB" and aren't done yet:

```go
tx := db.Transact(true)
defer tx.Rollback()

results, err := tx.NewQuery("todo").
    WhereLike("Task", "FlexDB").
    Where("Done", false).
    OrderBy("ID", false).
    Execute()

if err != nil {
    fmt.Println("Query failed! Time to debug:", err)
    return
}

for _, result := range results {
    todo := result.(*Todo)
    fmt.Printf("Still need to do: %s\n", todo.Task)
}
```

### Using Hooks

Let's add a hook to automatically set the creation time for new todos:

```go
db.RegisterHook("pre-set", func(tx *flexdb.Transaction, entityType string, entity flexdb.Entity) error {
    if entityType == "todo" {
        todo := entity.(*Todo)
        if todo.CreatedAt.IsZero() {
            todo.CreatedAt = time.Now()
        }
    }
    return nil
})
```

## 🤯 Complex Use Cases

### Multi-entity Transactions with Batch Operations

Let's create a project management system with projects and tasks:

```go
type Project struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

func (p *Project) GetID() string { return p.ID }
func (p *Project) SetID(id string) { p.ID = id }

type Task struct {
    ID        string    `json:"id"`
    ProjectID string    `json:"projectId"`
    Name      string    `json:"name"`
    Done      bool      `json:"done"`
    CreatedAt time.Time `json:"createdAt"`
}

func (t *Task) GetID() string { return t.ID }
func (t *Task) SetID(id string) { t.ID = id }

// Create a project with multiple tasks
tx := db.Transact(false)
defer func() {
    if !tx.committed {
        tx.Rollback()
    }
}()

project := &Project{ID: "p1", Name: "Learn Advanced FlexDB"}
err := tx.Set("project", project)
if err != nil {
    fmt.Println("Failed to create project:", err)
    return
}

tasks := []flexdb.Entity{
    &Task{ID: "t1", ProjectID: "p1", Name: "Master querying", Done: false},
    &Task{ID: "t2", ProjectID: "p1", Name: "Understand hooks", Done: false},
    &Task{ID: "t3", ProjectID: "p1", Name: "Become a transaction guru", Done: false},
}

err = tx.BatchSet("task", tasks)
if err != nil {
    fmt.Println("Failed to create tasks:", err)
    return
}

err = tx.Commit()
if err != nil {
    fmt.Println("Failed to commit transaction:", err)
    return
}

fmt.Println("Project and tasks created successfully!")
```

### Advanced Querying with Indexing

First, let's add an index to speed up our queries:

```go
db.AddIndex("task", "ProjectID")
```

Now, let's find all incomplete tasks for a specific project, ordered by creation time:

```go
tx := db.Transact(true)
defer tx.Rollback()

results, err := tx.NewQuery("task").
    Where("ProjectID", "p1").
    Where("Done", false).
    OrderBy("CreatedAt", false).
    Execute()

if err != nil {
    fmt.Println("Query failed:", err)
    return
}

fmt.Println("Incomplete tasks for project 'Learn Advanced FlexDB':")
for _, result := range results {
    task := result.(*Task)
    fmt.Printf("- %s (Created: %s)\n", task.Name, task.CreatedAt.Format(time.RFC822))
}
```

### Migrations

As your project evolves, you might need to change your data structure. Let's add a 'Priority' field to our tasks:

```go
db.AddMigration(1, 
    // Up migration
    func(tx *flexdb.Transaction) error {
        tasks := tx.GetAll("task")
        for _, t := range tasks {
            task := t.(*Task)
            task.Priority = "Medium" // Default priority
            err := tx.Set("task", task)
            if err != nil {
                return err
            }
        }
        return nil
    },
    // Down migration
    func(tx *flexdb.Transaction) error {
        // If needed, remove the Priority field here
        return nil
    },
)

// Run the migration
err := db.Migrate(1)
if err != nil {
    fmt.Println("Migration failed:", err)
    return
}

fmt.Println("Migration completed successfully!")
```

## 📚 API Reference

### Database

```go
type Database struct {
    cache *cache.Cache
    // other fields...
}

func NewDatabase(path string) (*Database, error)
func (db *Database) AddIndex(entityType, field string)
func (db *Database) RegisterHook(operation string, hook Hook)
func (db *Database) AddMigration(version int, up, down func(*Transaction) error)
func (db *Database) Migrate(targetVersion int) error
func (db *Database) Transact(readOnly bool) *Transaction
```

### Transaction

```go
type Transaction struct {
    committed bool
    // other fields...
}

func (tx *Transaction) Commit() error
func (tx *Transaction) Rollback()
func (tx *Transaction) Get(entityType string, id string) (Entity, bool)
func (tx *Transaction) GetAll(entityType string) []Entity
func (tx *Transaction) Set(entityType string, entity Entity) error
func (tx *Transaction) Delete(entityType string, id string) error
func (tx *Transaction) BatchSet(entityType string, entities []Entity) error
func (tx *Transaction) BatchDelete(entityType string, ids []string) error
func (tx *Transaction) NewQuery(entityType string) *Query
```

### Query

```go
type Query struct {}

func (q *Query) Where(field string, value interface{}) *Query
func (q *Query) WhereIn(field string, values []interface{}) *Query
func (q *Query) WhereLike(field string, value string) *Query
func (q *Query) Limit(limit int) *Query
func (q *Query) Offset(offset int) *Query
func (q *Query) OrderBy(field string, desc bool) *Query
func (q *Query) Execute() ([]Entity, error)
```

### Entity

```go
type Entity interface {
    GetID() string
    SetID(string)
}
```

### Hook

```go
// Hook is a function that can be registered to run before or after certain database operations within a transaction
type Hook func(tx *Transaction, entityType string, entity Entity) error
```

## 🎭 The FlexDB Philosophy

FlexDB is like a chameleon 🦎 – it adapts to your needs. Start simple, then scale up as your project grows. Whether you're building a simple script or a complex application, FlexDB is flexible enough to handle it all.

FlexDB uses a simple transaction model where changes are only visible within the transaction until committed. This provides a good balance between consistency and performance for many use cases.

Remember, with great power comes great responsibility. FlexDB gives you the tools, but it's up to you to wield them wisely. So go forth, young padawan, and may the code be with you! 🚀✨

Happy coding, and may your databases be forever consistent and your queries lightning fast! 💾⚡️
