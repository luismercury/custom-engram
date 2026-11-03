# Plan de Transición y Renombrado: `custom-engram` -> `my-second-brain` (`msb`)

**Fecha:** 18 de octubre de 2026  
**Estado:** Propuesta aprobada para evaluación — Pendiente de OK definitivo del usuario antes de ejecutar.

---

## 1. Resumen y Justificación

El proyecto actualmente denominado `custom-engram` fue concebido como una reimplementación nativa en Go (con SQLite zero-CGO y búsqueda híbrida) inspirada en el concepto de *engram*.

### ¿Por qué renombrar a "my-second-brain" / "msb"?
1. **Identidad Propia:** Deja de presentarse como un fork o parche de un proyecto externo para convertirse en un producto de memoria y gestión de conocimiento autónomo.
2. **Ergonomía de CLI:** El comando `msb` (3 teclas) es drásticamente más rápido y cómodo de usar en la terminal diaria (`msb save`, `msb search`, `msb context`) en comparación con `custom-engram` (13 teclas).
3. **Semántica Universal:** El concepto de *Second Brain* (Segundo Cerebro) es intuitivo y alineado con el propósito del sistema de persistir contexto y aprendizajes para agentes de IA y el usuario.

---

## 2. Evaluación de Viabilidad: SÍ (100% Viable)

La migración es completamente factible y de riesgo bajo si se implementa retrocompatibilidad y respaldo previo.

### Puntos Clave de Estabilidad:
- **Herramientas MCP sin romper:** El servidor MCP expone herramientas como `mem_save`, `mem_search`, `mem_context`, etc. Al usar el prefijo neutral `mem_`, **los agentes de IA (Antigravity, Claude, Pi, etc.) no experimentarán cambios en sus contratos ni en sus llamadas a herramientas.**
- **Base de Datos existente (126 KB):** Se creará un respaldo de seguridad previo de `~/.custom-engram/engram.db` y se copiará a `~/.msb/msb.db`.
- **Compatibilidad hacia atrás:** El resolver de proyectos soportará tanto `.msb/` como `.engram/`, evitando romper repositorios ya inicializados.

---

## 3. Estructura y Nomenclatura Final

| Elemento | Estado Actual | Estado Propuesto |
|---|---|---|
| **Nombre formal** | `custom-engram` | `my-second-brain` |
| **Siglas / CLI** | `custom-engram` | `msb` (alias opcional: `my-second-brain`) |
| **Carpeta del proyecto** | `C:\Users\Luis Mercury\proyectos\custom-engram` | `C:\Users\Luis Mercury\proyectos\my-second-brain` |
| **Go Module** | `module custom-engram` | `module my-second-brain` |
| **Punto de entrada Go** | `cmd/engram/main.go` | `cmd/msb/main.go` |
| **Binario resultante** | `bin/custom-engram.exe` | `bin/msb.exe` |
| **Directorio de datos** | `~/.custom-engram/engram.db` | `~/.msb/msb.db` |
| **Config local de repo** | `.engram/config.json` | `.msb/config.json` (con fallback a `.engram/`) |
| **Variables de entorno** | `ENGRAM_*` (`ENGRAM_DATA_DIR`, etc.) | `MSB_*` (con fallback a `ENGRAM_*`) |
| **Servidor MCP** | `"custom-engram"` / `"engram"` | `"msb"` |
| **Tags en Obsidian** | `- engram` | `- second-brain`, `- msb` |

---

## 4. Plan de Ejecución Paso a Paso (Para cuando el usuario dé el OK)

### Paso 1: Respaldo Preventivo de Datos
1. Crear una copia de seguridad timestamped de `~/.custom-engram/engram.db` en `~/.custom-engram/backup-pre-msb.db`.
2. Crear el nuevo directorio `~/.msb/` y copiar la base de datos a `~/.msb/msb.db`.

### Paso 2: Refactor del Código Go
1. Actualizar `go.mod` a `module my-second-brain`.
2. Renombrar carpeta `cmd/engram` a `cmd/msb`.
3. Actualizar los imports en todo el código fuente:
   - `custom-engram/internal/...` -> `my-second-brain/internal/...`.
4. Modificar strings de interfaz, ayuda y versión en `cmd/msb/main.go`.
5. En `internal/mcp/server.go`, registrar el servidor MCP como `msb`.

### Paso 3: Retrocompatibilidad en `resolver.go`
1. Modificar `internal/project/resolver.go`:
   - Buscar primero `.msb/config.json`.
   - Si no existe, buscar `.engram/config.json` como fallback.
2. Permitir que `msb init` cree `.msb/config.json`.

### Paso 4: Variables de Entorno con Fallback
1. Revisar lecturas de entorno para intentar primero `MSB_DATA_DIR`, `MSB_PROJECT`, `MSB_EMBED_MODEL`.
2. Si no están definidas, leer `ENGRAM_DATA_DIR`, `ENGRAM_PROJECT`, `ENGRAM_EMBED_MODEL`.

### Paso 5: Compilación y Verificación
1. Ejecutar tests unitarios: `go test ./...`.
2. Compilar binario: `go build -o bin/msb.exe ./cmd/msb`.
3. Validar comandos CLI:
   - `bin/msb.exe doctor`
   - `bin/msb.exe context`
   - `bin/msb.exe search "SQLite"`

### Paso 6: Actualización de Entornos de Agentes (MCP)
1. Actualizar `~/.gemini/config/mcp_config.json` para registrar el servidor `msb`.
2. Copiar/enlazar el ejecutable a `AppData\Local\msb\bin\msb.exe` (o mantener symlink/copia en el PATH).

### Paso 7: Migración de Metadatos del Proyecto en SQLite
1. Actualizar en `~/.msb/msb.db` la fila de `projects` donde `name = 'custom-engram'` para vincularla a `my-second-brain`, asegurando que no se pierda el contexto de este repositorio.

### Paso 8: Renombrado de Carpeta y Documentación
1. Renombrar la carpeta raíz a `C:\Users\Luis Mercury\proyectos\my-second-brain`.
2. Actualizar `README.md` reflejando la nueva marca `msb`.
