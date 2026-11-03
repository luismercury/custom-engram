# Servidor MCP — Especificación y Desarrollo

Directivas técnicas para modificar o extender el servidor Model Context Protocol (MCP) en `internal/mcp/`.

---

## 1. Protocolo y Pureza de `stdio`

* **Transporte:** stdio mediante JSON-RPC 2.0 (`github.com/mark3labs/mcp-go`).
* **Regla crítica de I/O:** **Nunca escribir a `os.Stdout`** dentro de paquetes internos o durante la ejecución del comando `mcp`. `stdout` está reservado exclusivamente para paquetes JSON-RPC. Cualquier traza, error o log debe enviarse estrictamente a `os.Stderr`.

---

## 2. Registro de Herramientas (`internal/mcp/server.go`)

Todas las herramientas MCP siguen el prefijo neutral `mem_`. Esto asegura retrocompatibilidad con cualquier agente y con la futura transición a `msb`.

### Herramientas principales:
* **Persistencia:** `mem_save`, `mem_update`, `mem_delete`, `mem_archive`, `mem_prune`.
* **Búsqueda e Inspección:** `mem_search`, `mem_context`, `mem_get_observation`, `mem_current_project`, `mem_stats`, `mem_doctor`.
* **Sesiones:** `mem_session_start`, `mem_session_end`, `mem_session_summary`.
* **Curación y Enriquecimiento:** `mem_pin`, `mem_unpin`, `mem_suggest_topic_key`, `mem_capture_passive`, `mem_judge`, `mem_compare`, `mem_save_prompt`.

---

## 3. Convenciones para Agregar/Modificar Herramientas

1. **Definición tipada:** Declarar parámetros claros con descripciones explicativas en `registerTools()`.
2. **Manejo de errores:** Devolver errores usando `mcp.NewToolResultError(msg)` en lugar de entrar en pánico (`panic`).
3. **Resolución de proyecto:** Usar siempre `s.resolveProject(req.Project)` para respetar la precedencia configurada.
4. **Pruebas:** Incorporar tests correspondientes en `internal/mcp/server_test.go`.
