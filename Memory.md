# BH BadHarness v2 — Memory

## Memory Architecture

BH BadHarness v2 uses persistent memory to:
- Remember who the user is
- Remember preferences and environment details
- Reuse procedures across turns
- Track decisions and outcomes across sessions

## Memory Store

### MemoryEntry
```go
type MemoryEntry struct {
    Key       string // unique key for this memory
    Label     string // human-readable label
    Value     string // stored value
    Source    string // where the entry came from (user, harness, skills, etc.)
    Timestamp time.Time // when the entry was created
}
```

### MemoryStore
```go
type MemoryStore struct {
    mu       sync.Mutex
    dir      string // directory where memory files live
    store    map[string]string // key -> JSON-serialized entry
    loaded   bool // whether data was loaded from disk
}
```

- One JSON file per key in `DataDir`
- `load()` — reads data from disk on startup
- Persists as JSON in `DataDir`

## Memory Lifecycle

1. **Startup** — `NewMemoryStore(dir)` creates store, calls `load()` to read existing data
2. **During operation** — memory entries are added as the agent runs
3. **Storage** — each key has a JSON file in `DataDir`
4. **On change** — memory is persisted
5. **On shutdown** — memory is saved

## Configuration

Memory directory is configured via:
- `DataDir` in `HarnessConfig`
- Default: under user cache (`os.UserCacheDir()`)

## Integration with Agent Loop

The `Harness` holds a `*MemoryStore` and:
- Loads memory on startup
- Adds entries as the agent runs
- Persists memory on change
- Saves memory on shutdown

## Data Format

Memory entries are stored as JSON files:
```json
{
    "key": "string",
    "label": "string",
    "value": "string",
    "source": "string",
    "timestamp": "time"
}
```

## Memory API

- `NewMemoryStore(dir string)` — creates and loads store
- `load()` — loads data from disk
- Entries are keyed by string for fast lookup
- Thread-safe with `sync.Mutex`
