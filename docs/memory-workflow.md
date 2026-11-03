# Protocolo de Memoria para Agentes Clientes

Guía de referencia para configurar y gobernar agentes de IA (Antigravity, Claude, Cursor, Pi, etc.) que consumen Custom Engram como su sistema de memoria.

---

## 1. Ciclo de Vida de una Sesión

Un agente disciplinado debe ejecutar el siguiente ciclo en cada sesión de trabajo:

```text
[Inicio de Sesión]
       │
       ▼
1. mem_session_start(goal)     --> Registra el objetivo principal de la tarea
       │
       ▼
2. mem_context()               --> Carga decisiones previas, sesiones recientes y contexto
       │
       ▼
3. mem_search(query)           --> Consulta memoria específica sobre el módulo a modificar
       │
[Trabajo Activo]
       │
       ▼
4. mem_save(...)               --> Registra decisiones, fixes críticos y aprendizajes
       │
[Cierre de Sesión]
       │
       ▼
5. mem_session_end(summary)    --> Cierra la sesión con un resumen de logros
```

---

## 2. Estándar de Guardado con `mem_save`

### Campos Requeridos y Sugeridos:
* `title`: Breve y descriptivo (ej: `Uso de modernc.org/sqlite para Zero-CGO en Windows`).
* `type`: Categoría estandarizada:
  * `decision` (elecciones arquitectónicas o de diseño)
  * `bugfix` (solución de errores no triviales)
  * `architecture` (estructuras, flujos o límites del sistema)
  * `discovery` (hallazgos sobre dependencias o entorno)
  * `pattern` (convenciones de código reutilizables)
  * `config` (ajustes de configuración relevantes)
  * `preference` (directivas de usuario o proyecto)
* `scope`: `project` (por defecto) | `personal` | `global`
* `topic_key`: Clave jerárquica normalizada (ej: `architecture/sqlite`, `build/zero-cgo`).

### Formato recomendado para `content`:
```markdown
**What:** Qué cambio o decisión se tomó.
**Why:** Razón técnica o requerimiento que motivó la decisión.
**Where:** Archivos, paquetes o funciones involucradas.
**Learned:** Regla o consideración a recordar en futuras sesiones.
```
