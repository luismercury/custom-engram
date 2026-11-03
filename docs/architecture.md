# Arquitectura y Reglas Técnicas — Custom Engram

Este documento contiene las especificaciones internas de arquitectura, almacenamiento y restricciones del motor `custom-engram`.

---

## 1. Zero-CGO (Regla Innegociable)

* **Restricción estricta:** No agregar dependencias de C (`gcc`, `clang`, `MinGW`).
* **Driver SQLite:** Se utiliza exclusivamente `modernc.org/sqlite`.
* **Propósito:** Permitir compilación instantánea y ejecución portable en Windows sin toolchains externas.

---

## 2. Base de Datos SQLite

* **Ubicación por defecto:** `~/.custom-engram/engram.db` (configurable vía `ENGRAM_DATA_DIR`).
* **Pragmas obligatorios:**
  ```sql
  PRAGMA journal_mode = WAL;
  PRAGMA busy_timeout = 5000;
  PRAGMA foreign_keys = ON;
  ```
* **Esquema principal:**
  * `projects`: Registro de proyectos (`id`, `name`, `root_path`).
  * `sessions`: Sesiones de trabajo (`id`, `project_id`, `goal`, `summary`, `status`).
  * `observations`: Memorias persistentes (`id`, `project_id`, `session_id`, `type`, `scope`, `title`, `content`, `topic_key`, `is_pinned`, `is_archived`).
  * `observations_fts`: Tabla virtual FTS5 (`porter unicode61`). Sincronizada automáticamente vía triggers (`AFTER INSERT`, `AFTER UPDATE`, `AFTER DELETE`).
  * `embeddings`: Vectores BLOB asociados a las observaciones.

---

## 3. Búsqueda Híbrida Inteligente

* **Motor:** `internal/search/hybrid.go`
* **Estrategia:** Combina resultados de FTS5 (BM25) con similitud coseno de embeddings vectoriales mediante **Reciprocal Rank Fusion (RRF)** con $k=60$.
* **Embeddings:** Vía Ollama local (`nomic-embed-text` por defecto) o fallback automático a FTS5 si Ollama no está disponible o falla.

---

## 4. Resolución de Proyectos

* **Motor:** `internal/project/resolver.go`
* **Orden de precedencia estricto:**
  1. Flag explícito CLI (`--project <nombre>`)
  2. Variable de entorno (`ENGRAM_PROJECT`)
  3. Archivo `.engram/config.json` en el árbol de directorios
  4. Raíz del repositorio Git más cercano
  5. Directorio de trabajo actual (`CWD`)
