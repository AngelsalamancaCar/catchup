# Project Mapper

Herramienta local para mapear y priorizar actividades: un cuestionario guiado
alimenta un motor de scoring que produce una Matriz de Impacto/Esfuerzo, una
Matriz Eisenhower, un mapa organizacional (swimlanes + treemap) y un timeline
de entregables — todo como SVG generado en el servidor, sin frameworks de
JavaScript ni base de datos.

## Qué es y qué no es

- Es un binario único que corre en tu propia máquina y sirve una página web
  en `http://127.0.0.1:8787`.
- **No es un servidor de red.** Escucha solo en `127.0.0.1` (loopback):
  ninguna otra máquina en tu red, ni internet, puede alcanzarlo. Cerrar la
  ventana de la consola detiene todo — no queda nada corriendo en segundo
  plano ni instalado como servicio.
- No requiere internet después de la instalación inicial, ni motor de base
  de datos: todos los datos viven en un único archivo JSON junto al binario.

## Cómo ejecutarlo

1. Descarga el binario para tu sistema operativo (ver "Empaquetado" abajo si
   necesitas compilarlo) y colócalo en una carpeta junto con una carpeta
   `data/` (puede estar vacía).
2. Haz doble click (Windows) o ejecútalo desde una terminal:
   ```bash
   ./projectmapper-windows-amd64.exe
   # o en mac/linux:
   ./projectmapper-darwin-arm64
   ./projectmapper-linux-amd64
   ```
3. Se abre automáticamente el navegador en `http://127.0.0.1:8787`. Si no se
   abre solo, entra a esa dirección manualmente.
4. Para detenerlo, cierra la ventana de la consola (o Ctrl+C).

Flags disponibles:

| Flag | Default | Qué hace |
|---|---|---|
| `-port` | `8787` | Puerto de escucha (siempre en `127.0.0.1`) |
| `-data` | `data/projects.json` | Ruta al archivo JSON de datos |
| `-seed` | (apagado) | Si `-data` no existe todavía, lo inicializa con `data/seed.json` (nunca sobreescribe datos reales) |

## Dónde viven los datos

Todo el portafolio (secciones, pesos de scoring, umbrales y actividades) es
un único archivo JSON en la ruta que indique `-data` (por defecto
`data/projects.json`). Los scores (Impacto, Esfuerzo, Urgencia, Importancia,
Incertidumbre) **no se guardan**: se recalculan siempre desde las respuestas
del cuestionario y los pesos configurados, así que cambiar un peso en
Configuración reclasifica todo el portafolio al instante, sin datos
desactualizados.

## Cómo respaldar

Configuración → "Descargar backup (.json)" (o `POST /export/backup`) crea una
copia timestamped del JSON de datos en `export/backup_YYYYMMDD_HHMMSS.json` y
la descarga. Además, `export/` también recibe una copia de cada Excel
exportado (`portfolio_YYYYMMDD.xlsx`). Como todo es un archivo plano, un
backup manual (copiar el JSON de datos) también sirve.

## Exportar

- **Excel** (Configuración → "Descargar Excel"): un `.xlsx` con hojas
  Actividades (todas las respuestas + scores + cuadrantes), Deliverables,
  Matriz I/E y Eisenhower (con scatter chart nativo y líneas de umbral), y
  Resumen por sección.
- **SVG de cada diagrama** (Configuración → enlaces de "Diagramas en SVG",
  o `GET /export/svg/{view}` con `view` en `impact-effort`, `eisenhower`,
  `org-swimlanes`, `org-treemap`, `timeline`): el mismo SVG que ves en
  pantalla, como archivo descargable.

## Cargar actividades en masa desde Excel

Configuración → "Cargar actividades desde Excel" es el camino para capturar
varias actividades sin pasar por el cuestionario una por una:

1. **Descargar template** (`GET /import/template.xlsx`): un `.xlsx` con la hoja
   `Actividades` (una fila por actividad, con desplegables que traen las
   opciones exactas del cuestionario y las secciones configuradas), la hoja
   `Entregables`, una hoja `Gantt` que se calcula sola y una hoja
   `Instrucciones` con el glosario de cada columna.
2. **Llenarlo**. Cada bloque del cuestionario tiene su banda de color
   (A Identificación, B Impacto, C Esfuerzo, D Urgencia/importancia,
   F Información) y todas las celdas de opción cerrada son desplegables: las
   preguntas 1–5 se eligen con su ancla incluida ("4 — Apoya un objetivo
   estratégico principal"), no como número suelto. La columna
   **¿Fila completa?** —calculada, la ignora el import— marca en verde/ámbar si
   a la fila le falta algún obligatorio, y los obligatorios vacíos se resaltan
   en la propia celda. En `Entregables`, la columna `Actividad` es un desplegable
   con los nombres que escribiste en la hoja anterior y "Depende de" con los
   entregables ya capturados (el ID `DLV-NNN` lo asigna Catchup al importar).
3. **Subirlo** (`POST /import/xlsx`, campo `file`). La carga es **todo-o-nada**:
   si alguna fila tiene un error no se escribe nada y la página lista cada
   problema con hoja, fila y columna. Las filas de ejemplo del template
   (prefijo `EJEMPLO`) se omiten solas, así que se pueden dejar.

El Gantt es un gráfico de barras apiladas nativo de Excel: la hoja `Gantt` lee
`Entregables` con fórmulas (desplazamiento y duración en días desde el inicio
del portafolio) y la primera serie va sin relleno, así la barra visible empieza
en la fecha de inicio de cada entregable. La columna `Inicio` existe solo para
ese gráfico — Catchup guarda la fecha `Fin` como fecha del entregable.

Como el import busca la fila de encabezados en las primeras filas de la hoja y
mapea las columnas por nombre —aceptando tanto las etiquetas de los desplegables
como las claves internas (`Project`, `breach`, `60-120`…)—, un archivo bajado con
"Descargar Excel" también se puede editar y volver a subir. Cada carga **crea**
actividades nuevas: no actualiza ni reemplaza las existentes.

El template se genera al momento de cada descarga, así que no hay archivo que
mantener ni reemplazar: al actualizar el binario, el siguiente "Descargar
template" ya trae la versión nueva (y las secciones que tengas configuradas en
ese momento).

## Cómo importar un backlog existente

Si ya tienes una lista de actividades en Excel, `scripts/import_backlog.py`
(opcional, requiere Python + `openpyxl`) la convierte a un JSON importable —
ver `scripts/README.md` para el formato de columnas esperado. El JSON
resultante se sube en Configuración → "Importar backlog" (o
`POST /import` con el archivo en el campo `file`).

Las actividades importadas por script no traen las respuestas detalladas de
Impacto/Esfuerzo/Urgencia (un backlog típico no las tiene), así que aparecen
marcadas con confianza mínima y el badge "necesita info" en el Eisenhower
hasta que se complete el cuestionario real.

Si no tienes Python, también puedes crear las actividades a mano vía el
cuestionario — el script es una comodidad opcional, no un requisito (§2.1 de
la propuesta original).

## Atajo en el escritorio (Windows)

```powershell
./scripts/install-shortcut.ps1
```

Compila, copia el binario a `%LOCALAPPDATA%\Catchup\catchup.exe` y crea el atajo
**Catchup** en el escritorio y en el menú Inicio (así aparece al escribir
"catchup" en el buscador de Windows). Doble click abre el navegador en
`http://127.0.0.1:8787`.

- **Los datos viven en `%LOCALAPPDATA%\Catchup\data\projects.json`** y los
  exports en `%LOCALAPPDATA%\Catchup\export\` — fuera del repo, así que
  recompilar, hacer `git clean` o borrar `dist/` no los toca. El atajo pasa esa
  ruta absoluta, no depende del directorio desde donde se lance.
- **Para detener la app, cerrá la ventana de consola** que queda minimizada en
  la barra de tareas (o Ctrl+C en ella). El atajo la abre minimizada justamente
  para que sirva de interruptor.
- **Un segundo doble click no rompe nada**: si ya hay una instancia corriendo en
  ese puerto, la nueva solo abre el navegador en la que ya está viva y sale.
- Se puede volver a correr después de cada rebuild — es idempotente y nunca
  sobreescribe `projects.json`. Si la app está abierta, avisa que la cierres
  primero (Windows no deja reemplazar un `.exe` en uso).

Opciones útiles:

| Flag | Qué hace |
|---|---|
| `-NoBuild` | Reusa `dist/` como está, sin recompilar |
| `-Name <texto>` + `-InstallDir <ruta>` + `-Port <n>` | Segundo portafolio independiente, con su propio atajo y datos |
| `-Remove` | Borra los atajos y deja los datos intactos |
| `-Remove -Purge` | Borra también el directorio de instalación (pide confirmación) |

El ícono es `assets/catchup.ico`, ya versionado en el repo; se regenera con
`go run ./scripts/mkicon` si se cambia la paleta de `internal/svg`.

## Empaquetado (compilar los binarios)

```bash
./build.sh      # o build.ps1 en PowerShell
```

Compila `dist/projectmapper-{windows-amd64.exe, darwin-arm64, linux-amd64}`
con `CGO_ENABLED=0` y `-ldflags "-s -w"` (sin símbolos de debug, más
livianos). Cada binario es completamente standalone: no necesita Go, ni
ninguna librería del sistema, instalados en la máquina destino — solo copia
el ejecutable junto a una carpeta `data/`.

## Preguntas frecuentes

**¿Esto expone mis datos en la red de mi oficina/casa?**
No. `127.0.0.1` es la dirección de loopback: solo procesos en tu misma
máquina pueden conectarse. Otro equipo en la misma red, aunque conozca tu IP,
no puede alcanzar el puerto 8787 de tu máquina a través de esa dirección.

**¿Necesito instalar algo más (base de datos, Node, Python)?**
No para el uso normal: el binario ya trae todo lo que necesita. Python solo
hace falta si vas a usar el script opcional de importación de backlog.

**¿Qué pasa si cierro la consola sin querer?**
El servidor se detiene y tus datos ya guardados (cada cambio se escribe al
JSON inmediatamente, no hay un botón de "guardar" separado) quedan intactos.
Solo vuelve a ejecutar el binario para seguir donde quedaste.

**¿Puedo tener varios portafolios distintos?**
Sí, corriendo el binario con un `-data` distinto para cada uno (y, si los
corres al mismo tiempo, un `-port` distinto para cada instancia). Con atajos:
`./scripts/install-shortcut.ps1 -Name "Catchup 2025" -InstallDir "$env:LOCALAPPDATA\Catchup-2025" -Port 8788`
crea un segundo ícono con sus propios datos.
