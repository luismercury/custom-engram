# AGENTS.md — Custom Engram

Sistema local-first de memoria persistente para agentes de IA en Go con búsqueda híbrida y servidor MCP.

---

## 1. Reglas Innegociables

1. **Zero-CGO Estricto:** Prohibido agregar dependencias C (`gcc`/`clang`/`MinGW`). SQLite se maneja únicamente con `modernc.org/sqlite`.
2. **Pureza de `stdio` en MCP:** Nunca escribir logs ni prints a `os.Stdout` en paquetes internos; corrompe el canal JSON-RPC. Usar exclusivamente `os.Stderr`.

---

## 2. Comandos Esenciales

```bash
# Compilar binario en Windows
go build -o bin/custom-engram.exe ./cmd/engram

# Ejecutar pruebas unitarias
go test ./...

# Diagnóstico de salud, SQLite y embeddings
./bin/custom-engram.exe doctor

# Consultar contexto activo del proyecto
./bin/custom-engram.exe context
```

---

## 3. Índice de Documentación Especializada

Consulta estos documentos según la tarea a realizar:

| Si tu tarea implica... | Consulta este documento |
|---|---|
| Modificar base de datos, FTS5, embeddings o resolver | [`docs/architecture.md`](docs/architecture.md) |
| Alterar o agregar herramientas del servidor MCP | [`docs/mcp-server.md`](docs/mcp-server.md) |
| Protocolo y buenas prácticas de uso de memoria `mem_*` | [`docs/memory-workflow.md`](docs/memory-workflow.md) |
| Evaluar o revisar la transición futura a `msb` | [`PLAN_RENOMBRADO_MSB.md`](PLAN_RENOMBRADO_MSB.md) |
| Reconstruir el proyecto o respaldar la especificación | [`RECONSTRUCTION_BLUEPRINT.md`](RECONSTRUCTION_BLUEPRINT.md) |
| Catálogo general de herramientas CLI y MCP | [`README.md`](README.md) |

---

## 4. Estructura Rápida de Paquetes

* `cmd/engram/`: CLI principal y subcomandos (`mcp`, `save`, `search`, `context`, etc.).
* `internal/store/`: Base de datos SQLite (WAL, DDL, queries, migraciones).
* `internal/search/`: Búsqueda híbrida FTS5 BM25 + embeddings Ollama (RRF).
* `internal/mcp/`: Servidor Model Context Protocol y registro de herramientas `mem_*`.
* `internal/project/`: Resolver determinista de proyectos.
* `internal/exporter/`: Exportación a Markdown (Obsidian) y JSON.
