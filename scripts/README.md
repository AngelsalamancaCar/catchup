# scripts/import_backlog.py

Convierte un backlog Excel existente en un JSON que Project Mapper puede
importar. Es **opcional**: la app funciona sin Python; este script solo evita
tener que crear cada actividad a mano vía el cuestionario cuando ya existe
una lista en Excel.

## Instalación

```bash
pip install -r requirements.txt
```

Requiere Python 3.9+ y `openpyxl` (única dependencia).

## Formato de entrada esperado

Un `.xlsx` con una fila de encabezados y estas columnas (nombres flexibles,
no sensibles a mayúsculas ni acentos):

| Columna lógica | Encabezados aceptados |
|---|---|
| Nombre (obligatoria) | `nombre`, `name`, `actividad`, `proyecto` |
| Tipo | `tipo`, `type` |
| Sección | `sección`, `seccion`, `section`, `área`, `area` |
| Owner | `owner`, `responsable`, `dueño`, `dueno` |
| Deadline | `deadline`, `fecha límite`, `fecha limite`, `vencimiento`, `due` |

Solo **Nombre** es obligatoria; el resto queda vacío/sin deadline si falta.

Valores de **Tipo** reconocidos (si no coincide con ninguno, se importa como
`AdHoc` y se avisa por consola): `project`/`proyecto`, `workstream`/
`iniciativa`, `recurring`/`recurrente`, `adhoc`/`ad-hoc`/`tarea`.

**Deadline** acepta celdas de fecha nativas de Excel o texto en
`YYYY-MM-DD`, `DD/MM/YYYY` o `DD-MM-YYYY`.

## Uso

```bash
python import_backlog.py backlog.xlsx -o import.json
python import_backlog.py backlog.xlsx --sheet "Hoja1"
```

Por defecto escribe `<nombre-del-xlsx>.json` junto al archivo de entrada.

## Qué contienen las actividades importadas

El backlog típico no trae las secciones B–F del cuestionario (Impacto,
Esfuerzo, Urgencia/Importancia, información). El script las completa con
valores neutros y, a propósito, marca `f1` (confianza) en 1 y `f2` con esos
mismos campos como conjetura — así cada actividad importada aparece con el
badge "ℹ necesita info" en el Eisenhower hasta que alguien complete el
cuestionario real con las respuestas correctas. `f3` queda con una nota
explicando esto.

## Cómo importar el JSON generado

Dos formas, ambas soportadas por la app (no es necesario elegir en el momento
de correr el script):

1. **Recomendada — vía la app:** Configuración → "Importar backlog" → sube
   el JSON (o `curl -F file=@import.json http://127.0.0.1:8787/import`).
   Agrega las actividades a la instalación existente sin tocar secciones,
   pesos ni umbrales ya configurados.
2. **Reemplazando el archivo de datos:** el JSON generado tiene la forma
   completa del Store (`sections`/`weights`/`thresholds`/`activities`), así
   que también sirve como archivo `-data` directo. **Cuidado:** esto
   reemplaza *todo* el archivo, incluyendo cualquier configuración o
   actividad que ya existiera — úsalo solo en una instalación nueva/vacía, o
   después de respaldar con Configuración → "Descargar backup".
