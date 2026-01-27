# PostgreSQL Database Package

This package provides database connection and schema management using pgx/v5 and sqlc for type-safe SQL queries.

## Setup

### Environment Variables

Set the following environment variables for database connection:

```bash
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=sforce
DB_SSLMODE=disable  # or 'require', 'verify-full', etc.
```

### Initializing the Schema

To initialize the database schema, you can either:

1. **Run the SQL file directly:**
   ```bash
   psql -U postgres -d sforce -f schema/postgres/migrations/001_initial_schema.sql
   ```

2. **Use the Go code:**
   ```go
   import (
       "github.com/natserract/sforce/schema/postgres"
       "go.uber.org/zap"
   )
   
   logger, _ := zap.NewProduction()
   cfg := postgres.NewConfig()
   db, err := postgres.New(cfg, logger)
   if err != nil {
       log.Fatal(err)
   }
   defer db.Close()
   
   // Initialize schema from file
   ctx := context.Background()
   err = db.InitSchemaFromFile(ctx, "schema/postgres/migrations/001_initial_schema.sql")
   if err != nil {
       log.Fatal(err)
   }
   ```

## Database Schema

### Tables

1. **folders** - Stores folder hierarchy (parent and child folders)
2. **data_extensions** - Stores data extension records
3. **data_retention_properties** - Stores retention settings for data extensions
4. **message_queue** - Durable message queue for async processing
5. **message_history** - Audit log for message processing

### Relationships

- `folders.parent_id` → `folders.id` (self-referencing foreign key)
- `data_extensions.category_id` → `folders.id`
- `data_retention_properties.data_extension_id` → `data_extensions.id`
- `message_history.message_id` → `message_queue.id`

## SQLC Code Generation

This package uses [sqlc](https://sqlc.dev/) to generate type-safe Go code from SQL queries.

### Generating Code

After adding or modifying SQL queries in `schema/postgres/queries/`, run:

```bash
sqlc generate
```

This will regenerate the code in `schema/postgres/gen/`.

### Query Files

- `folders.sql` - Folder CRUD operations
- `data_extensions.sql` - Data extension operations
- `data_retention_properties.sql` - Retention properties operations
- `message_queue.sql` - Message queue operations
- `message_history.sql` - Message history operations

### Using Generated Code

```go
import (
    "github.com/natserract/sforce/schema/postgres"
    "github.com/natserract/sforce/schema/postgres/gen"
)

// Create database connection
db, _ := postgres.New(postgres.NewConfig(), logger)
defer db.Close()

// Create queries instance
queries := gen.New()

// Use generated queries
folder, err := queries.GetFolderByID(ctx, db.Pool(), "1708")
```

## Migrations

Migration files are stored in `schema/postgres/migrations/`. To apply migrations:

1. Run migrations sequentially in order
2. Use a migration tool like `golang-migrate` or `atlas` for production

