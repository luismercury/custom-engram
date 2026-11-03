# Blueprint Maestro de Reconstrucción: Custom Engram

> **Propósito:** Este documento es una especificación técnica autónoma y autocontenida que contiene toda la arquitectura, esquemas de datos, algoritmos, contratos MCP y directivas necesarias para reconstruir el proyecto `custom-engram` desde cero (mediante un desarrollador o un agente de IA), sin necesidad de acceder al repositorio original.

---

## 1. Visión General y Principios de Diseño

* **Nombre del Sistema:** `custom-engram` (o alias/futuro: `my-second-brain` / `msb`).
* **Objetivo:** Motor de memoria persistente local-first para agentes de IA (Antigravity, Claude Code, Cursor, Pi, Roo Code, etc.) mediante protocolo MCP sobre `stdio`.
* **Stack Principal:** Go 1.22+ / 1.26+.
* **Principios Innegociables:**
  1. **Zero-CGO Estricto:** Base de datos embebida usando `modernc.org/sqlite`. Debe compilar nativamente en Windows sin GCC/MinGW ni toolchains externas.
  2. **Pureza de Transporte `stdio`:** `stdout` está reservado exclusivamente para mensajes JSON-RPC 2.0 del protocolo MCP. Todo log, advertencia o mensaje de depuración debe ir a `stderr`.
  3. **Búsqueda Híbrida con Fallback:** FTS5 BM25 léxico + embeddings vectoriales con Reciprocal Rank Fusion (RRF). Si el servicio de embeddings no está disponible, el sistema debe degradar graciosamente a FTS5 sin fallar.
  4. **Resolución Determinista:** Soporte multi-proyecto determinista que previene colisiones en directorios home (`$HOME`).

---

## 2. Dependencias (`go.mod`)

```go
module custom-engram

go 1.26.6

require (
    github.com/google/uuid v1.6.0
    github.com/mark3labs/mcp-go v0.11.0
    modernc.org/sqlite v1.36.0
)
```

---

## 3. Estructura de Directorios

```text
custom-engram/
├── cmd/
│   └── engram/
│       └── main.go            # Entrypoint CLI y despacho de subcomandos
├── internal/
│   ├── store/                 # SQLite, DDL, modelos, vectores y operaciones CRUD
│   │   ├── schema.go          # DDL, triggers FTS5 y pragmas
│   │   ├── models.go          # Structs de datos en Go
│   │   ├── store.go           # Métodos de acceso y queries a SQLite
│   │   └── vector.go          # Serialización LittleEndian y Similitud Coseno
│   ├── search/                # Motor de búsqueda y embeddings
│   │   ├── embedder.go        # Cliente HTTP para Ollama con fallback
│   │   └── hybrid.go          # Algoritmo Reciprocal Rank Fusion (RRF)
│   ├── project/               # Resolución de proyecto activo
│   │   └── resolver.go        # Precedencia: Flag > Env > Config > Git > Home
│   ├── mcp/                   # Servidor Model Context Protocol
│   │   └── server.go          # Implementación stdio y catálogo de herramientas mem_*
│   └── exporter/              # Exportación de datos
│       └── obsidian.go        # Generación de bóveda Markdown (Obsidian) y JSON
└── bin/                       # Directorio de binarios compilados
```

---

## 4. Esquema de Base de Datos y Almacenamiento (SQLite)

### Pragmas Obligatorios al Abrir Conexión:
```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
```

### DDL Completo:
```sql
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    root_path TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    goal TEXT,
    summary TEXT,
    status TEXT NOT NULL DEFAULT 'active', -- 'active' | 'completed'
    created_at DATETIME NOT NULL,
    ended_at DATETIME,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT,
    type TEXT NOT NULL DEFAULT 'discovery',
    scope TEXT NOT NULL DEFAULT 'project',
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    topic_key TEXT,
    is_pinned INTEGER NOT NULL DEFAULT 0,
    is_archived INTEGER NOT NULL DEFAULT 0,
    archived_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_obs_project ON observations(project_id);
CREATE INDEX IF NOT EXISTS idx_obs_topic ON observations(topic_key);
CREATE INDEX IF NOT EXISTS idx_obs_type ON observations(type);
CREATE INDEX IF NOT EXISTS idx_obs_created ON observations(created_at);

-- Tabla virtual FTS5 para búsqueda léxica
CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    id UNINDEXED,
    project_id UNINDEXED,
    title,
    content,
    topic_key,
    type,
    scope,
    tokenize = 'porter unicode61'
);

-- Triggers de sincronización automática FTS5
CREATE TRIGGER IF NOT EXISTS trig_obs_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts (id, project_id, title, content, topic_key, type, scope)
    VALUES (new.id, new.project_id, new.title, new.content, new.topic_key, new.type, new.scope);
END;

CREATE TRIGGER IF NOT EXISTS trig_obs_ad AFTER DELETE ON observations BEGIN
    DELETE FROM observations_fts WHERE id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS trig_obs_au AFTER UPDATE ON observations BEGIN
    UPDATE observations_fts SET 
        title = new.title,
        content = new.content,
        topic_key = new.topic_key,
        type = new.type,
        scope = new.scope
    WHERE id = old.id;
END;

-- Tabla de embeddings vectoriales
CREATE TABLE IF NOT EXISTS embeddings (
    observation_id TEXT PRIMARY KEY,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    vector BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE
);

-- Tabla de prompts de usuario
CREATE TABLE IF NOT EXISTS prompts (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);
```

---

## 5. Serialización de Vectores y Búsqueda Híbrida

### Serialización Little-Endian (`internal/store/vector.go`):
* Un vector `[]float32` se serializa en un slice de bytes `[]byte` donde cada float de 32 bits ocupa 4 bytes (`binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))`).
* La deserialización lee de a 4 bytes mediante `math.Float32frombits(binary.LittleEndian.Uint32(...))`.

### Similitud Coseno:
$$\text{CosineSimilarity}(A, B) = \frac{A \cdot B}{\|A\|_2 \cdot \|B\|_2}$$
Umbral mínimo semántico sugerido: $0.25$.

### Fusión Recíproca de Rangos (RRF):
$$RRF(d) = \sum_{m \in \{\text{FTS5}, \text{Vector}\}} \frac{1}{k + \text{rank}_m(d)}$$
donde la constante de suavizado es $k = 60.0$.
* **Fallback:** Si Ollama está apagado o falla el request HTTP a `http://localhost:11434/api/embeddings`, la búsqueda retorna directamente los resultados ordenados de FTS5 BM25.

---

## 6. Algoritmo de Resolución de Proyectos (`resolver.go`)

Precedencia para determinar el proyecto activo y su raíz:
1. **Flag CLI:** `--project <nombre>`
2. **Variable de Entorno:** `ENGRAM_PROJECT` (o `MSB_PROJECT`)
3. **Archivo de Configuración:** Búsqueda ascendente de `.engram/config.json` (o `.msb/config.json`)
4. **Repositorio Git:** Búsqueda ascendente de la carpeta `.git` (nombre = nombre del directorio raíz de Git)
5. **Detección de Directorio Home:** Si `CWD == $HOME`, asignar nombre `"global"` para evitar indexar todo el home del usuario erróneamente.
6. **Fallback CWD:** Directorio base actual (`filepath.Base(cwd)`).

---

## 7. Catálogo Completo de Herramientas MCP (20 Herramientas)

Todas expuestas a través de JSON-RPC sobre `stdio`:

1. `mem_save`: Guarda una observación (`title`, `content` con formato What/Why/Where/Learned, `type`, `scope`, `topic_key`, `project`).
2. `mem_update`: Actualiza campos puntuales de una memoria por `id` (UUID).
3. `mem_search`: Búsqueda híbrida (FTS5 + semántica) con filtro de `project` y `limit`.
4. `mem_context`: Recupera observaciones fijadas (`is_pinned`), observaciones recientes y estado de la última sesión.
5. `mem_session_start`: Inicia una sesión con `goal` y retorna su ID.
6. `mem_session_end`: Concluye la sesión activa asociando un `summary`.
7. `mem_session_summary`: Persiste o actualiza el resumen de sesión.
8. `mem_suggest_topic_key`: Sugiere claves jerárquicas normalizadas (ej: `architecture/sqlite-fts5`).
9. `mem_capture_passive`: Analiza texto con la sección `## Key Learnings:` y extrae observaciones automáticamente.
10. `mem_pin`: Marca `is_pinned = 1` para priorizar una memoria en el contexto.
11. `mem_unpin`: Marca `is_pinned = 0`.
12. `mem_save_prompt`: Registra un prompt de usuario en la tabla `prompts`.
13. `mem_get_observation`: Obtiene el contenido completo de una observación por su `id`.
14. `mem_delete`: Elimina permanentemente una observación por `id`.
15. `mem_current_project`: Informa el proyecto actual resuelto y su ruta raíz.
16. `mem_stats`: Métricas del sistema (total de proyectos, sesiones, memorias, embeddings).
17. `mem_doctor`: Diagnóstico de salud de SQLite, WAL y conexión con Ollama.
18. `mem_judge`: Evalúa y guarda relación entre dos memorias (`compatible`, `supersedes`, `contradicts`).
19. `mem_compare`: Persiste el resultado de una comparación semántica.
20. `mem_archive`: Oculta (`is_archived = 1`) o restaura una memoria.
21. `mem_prune`: Purga o archiva memorias antiguas con soporte para `--dry-run`.

---

## 8. Comandos CLI (`cmd/engram/main.go`)

* `custom-engram mcp`: Lanza el servidor stdio.
* `custom-engram save <title> <msg>`: Guardado rápido por CLI.
* `custom-engram search <query>`: Búsqueda híbrida en terminal.
* `custom-engram context`: Muestra el contexto actual del proyecto.
* `custom-engram export [--vault <path>] [--format obsidian|json]`: Exporta notas Markdown con frontmatter YAML y enlaces `[[topic_key]]`.
* `custom-engram init [name]`: Crea `.engram/config.json`.
* `custom-engram doctor`: Verifica integridad y conectividad.
* `custom-engram archive <id>`: Archiva una memoria.
* `custom-engram prune [--older-than <duration>] [--dry-run]`: Limpieza por antigüedad.

---

## 9. Prompt de Reconstrucción para LLM

Para reconstruir este proyecto en una máquina limpia con cualquier modelo de IA (Claude, GPT, Pi, Gemini), copia y pega el siguiente prompt junto con este archivo:

```text
Actúa como un Ingeniero Principal en Go.
Toma el documento 'RECONSTRUCTION_BLUEPRINT.md' como especificación técnica completa y obligatoria.
Reconstruye el proyecto 'custom-engram' implementando todos los paquetes en Go:
1. go.mod con dependencias Zero-CGO (modernc.org/sqlite, mark3labs/mcp-go, google/uuid).
2. internal/store (schema DDL con WAL/FTS5/triggers, models, vector float32 LittleEndian y queries).
3. internal/search (cliente Ollama embeddings y algoritmo Hybrid RRF k=60 con fallback).
4. internal/project (resolver determinista con home-detection).
5. internal/exporter (exportador Obsidian con frontmatter YAML y wikilinks).
6. internal/mcp (servidor stdio con las herramientas mem_* completas, garantizando cero output a stdout).
7. cmd/engram/main.go (CLI de subcomandos).

Asegúrate de que 'go test ./...' pase al 100% y 'go build -o bin/custom-engram.exe ./cmd/engram' compile limpiamente sin herramientas de C.
```
