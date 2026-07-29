package server

import (
	"fmt"
	"strings"
)

// Este archivo define, en un solo lugar, las columnas del template de Excel de
// carga masiva: encabezado, tipo de dato, valores válidos y ayuda. Tanto el
// generador del template (xlsx_template.go) como el parser del archivo subido
// (xlsx_import.go) se derivan de estas definiciones, para que agregar o
// cambiar una pregunta no requiera tocar dos listas paralelas.

// codedField describe un campo de opción cerrada: la clave que persiste el
// modelo y la etiqueta en español que ve el usuario. El import acepta ambas
// formas, así un archivo generado por /export/xlsx (que trae claves crudas)
// también se puede volver a subir.
type codedField struct {
	keys   []string
	labels []string
}

func (c codedField) label(key string) string {
	for i, k := range c.keys {
		if k == key {
			return c.labels[i]
		}
	}
	return key
}

// resolve traduce lo que haya en la celda (clave o etiqueta, sin importar
// mayúsculas, espacios ni acentos) a la clave canónica del modelo.
func (c codedField) resolve(raw string) (string, bool) {
	v := normalizeCell(raw)
	if v == "" {
		return "", false
	}
	for i, k := range c.keys {
		if v == normalizeCell(k) || v == normalizeCell(c.labels[i]) {
			return k, true
		}
	}
	return "", false
}

var accentFolder = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// normalizeCell deja el texto de una celda en forma comparable: sin espacios
// alrededor, en minúsculas y sin acentos ("Sí" y "si" son el mismo valor).
func normalizeCell(s string) string {
	return accentFolder.Replace(strings.ToLower(strings.TrimSpace(s)))
}

// Campos de opción cerrada. Las etiquetas son las mismas que usa el
// cuestionario en pantalla (activity_wizard.html.tmpl), para que quien llena
// el Excel vea exactamente las opciones que vería en la app.
var (
	typeCoded = codedField{
		keys:   []string{"Project", "Workstream", "Recurring", "AdHoc"},
		labels: []string{"Proyecto", "Workstream", "Actividad recurrente", "Ad-hoc"},
	}
	c1Coded = codedField{
		keys:   []string{"<5", "5-20", "20-60", "60-120", ">120"},
		labels: []string{"Menos de 5", "5 a 20", "20 a 60", "60 a 120", "Más de 120"},
	}
	c4Coded = codedField{
		keys:   []string{"none", "some", "blocking"},
		labels: []string{"Ninguna", "Algunas", "Bloqueantes"},
	}
	d2Coded = codedField{
		keys:   []string{"nothing", "friction", "escalation", "breach"},
		labels: []string{"Nada", "Fricción menor", "Escalamiento", "Incumplimiento o pérdida"},
	}
	d3Coded = codedField{
		keys:   []string{"mine", "shared", "other"},
		labels: []string{"Míos", "Compartidos", "De otra persona o área"},
	}
	d4Coded = codedField{
		keys:   []string{"yes", "partial", "no"},
		labels: []string{"Sí", "Parcialmente", "No"},
	}
	statusCoded = codedField{
		keys:   []string{"not_started", "in_progress", "at_risk", "done"},
		labels: []string{"No iniciado", "En progreso", "En riesgo", "Hecho"},
	}
)

// colKind es el tipo de dato de una columna del template.
type colKind int

const (
	colText    colKind = iota // texto libre
	colMulti                  // varios valores separados por ";"
	colScale                  // entero 1-5 con anclas
	colCoded                  // opción cerrada (codedField)
	colSection                // sección organizacional (lista dinámica del store)
	colDate                   // fecha
	colF2                     // claves marcadas como conjetura, separadas por ";"
)

// Grupos de columnas: dan la banda de color superior de la hoja y agrupan el
// glosario, siguiendo las secciones del cuestionario.
const (
	groupIdent   = "A · Identificación"
	groupImpact  = "B · Impacto"
	groupEffort  = "C · Esfuerzo"
	groupUrgency = "D · Urgencia e importancia"
	groupInfo    = "F · Suficiencia de información"
	groupDlv     = "E · Entregables"
)

// xlsxColumn describe una columna del template de carga masiva.
type xlsxColumn struct {
	Header   string // encabezado (clave de mapeo al importar)
	Question string // pregunta completa del cuestionario, para tooltip/glosario
	Group    string // bloque del cuestionario al que pertenece
	Kind     colKind
	Required bool
	Width    float64
	Anchor   string     // clave de anchors() cuando Kind == colScale
	Coded    codedField // cuando Kind == colCoded
	Help     string     // nota extra para el glosario de la hoja Instrucciones
}

// listName es el nombre de la lista de valores válidos en la hoja Listas, o ""
// si la columna es de texto libre. Varias columnas comparten lista (todas las
// escalas 1-5 usan la misma).
func (c xlsxColumn) listName() string {
	switch c.Kind {
	case colScale:
		return "Escala 1-5"
	case colSection:
		return "Sección"
	case colCoded:
		return c.Header
	default:
		return ""
	}
}

// scaleOption es una opción del desplegable de una pregunta 1-5: el número más
// su ancla, para que quien llena el Excel no tenga que recordar qué significa
// cada punto de la escala. El import se queda solo con el número inicial.
func scaleOption(o ScaleOption) string {
	return fmt.Sprintf("%d — %s", o.Value, o.Anchor)
}

// listValues son los valores que alimentan el dropdown de la columna.
func (c xlsxColumn) listValues(sections []string) []string {
	switch c.Kind {
	case colScale:
		opts := anchors(c.Anchor)
		out := make([]string, 0, len(opts))
		for _, o := range opts {
			out = append(out, scaleOption(o))
		}
		return out
	case colSection:
		return sections
	case colCoded:
		return c.Coded.labels
	default:
		return nil
	}
}

// validValuesText describe los valores aceptados, para el glosario y para el
// mensaje de ayuda que Excel muestra al seleccionar la celda.
func (c xlsxColumn) validValuesText() string {
	switch c.Kind {
	case colScale:
		var parts []string
		for _, o := range anchors(c.Anchor) {
			parts = append(parts, fmt.Sprintf("%d = %s", o.Value, o.Anchor))
		}
		return strings.Join(parts, " · ")
	case colCoded:
		return strings.Join(c.Coded.labels, " · ")
	case colSection:
		return "una de las secciones configuradas en Catchup"
	case colDate:
		return "fecha (AAAA-MM-DD)"
	case colMulti, colF2:
		return "varios valores separados por punto y coma (;)"
	default:
		return "texto libre"
	}
}

// activityColumns son las columnas de la hoja Actividades. Los encabezados
// coinciden con los que emite /export/xlsx, de modo que un export se puede
// editar y volver a subir sin renombrar nada.
func activityColumns() []xlsxColumn {
	return []xlsxColumn{
		{Header: "Nombre", Question: "A1. Nombre de la actividad", Group: groupIdent, Kind: colText, Required: true, Width: 34,
			Help: "Una fila = una actividad. Si está vacío, la fila se ignora."},
		{Header: "Tipo", Question: "A2. Tipo de actividad", Group: groupIdent, Kind: colCoded, Coded: typeCoded, Required: true, Width: 20,
			Help: "Proyecto y Workstream se muestran por defecto en Impacto/Esfuerzo; recurrente y ad-hoc en Eisenhower."},
		{Header: "Owner", Question: "A3. Owner (responsable)", Group: groupIdent, Kind: colText, Required: true, Width: 18},
		{Header: "Sección", Question: "A4. Sección responsable", Group: groupIdent, Kind: colSection, Required: true, Width: 18,
			Help: "Si la sección no existe en Catchup, la actividad se importa igual y aparece en el carril \"Sin sección\"."},
		{Header: "Involucradas", Question: "A5. Otras secciones involucradas", Group: groupIdent, Kind: colMulti, Width: 26},
		{Header: "Descripción", Question: "A6. Descripción / objetivo", Group: groupIdent, Kind: colText, Width: 40},

		{Header: "B1", Group: groupImpact, Question: "B1. Contribución a objetivos estratégicos", Kind: colScale, Anchor: "b1", Required: true, Width: 30},
		{Header: "B2", Group: groupImpact, Question: "B2. Nº de stakeholders/usuarios afectados", Kind: colScale, Anchor: "b2", Required: true, Width: 30},
		{Header: "B3", Group: groupImpact, Question: "B3. Efecto financiero o de valor", Kind: colScale, Anchor: "b3", Required: true, Width: 30},
		{Header: "B4", Group: groupImpact, Question: "B4. Consecuencia de no hacerlo", Kind: colScale, Anchor: "b4", Required: true, Width: 30},

		{Header: "C1", Group: groupEffort, Question: "C1. Esfuerzo estimado (persona-días)", Kind: colCoded, Coded: c1Coded, Required: true, Width: 16},
		{Header: "C2", Group: groupEffort, Question: "C2. Nº de secciones/equipos que deben coordinar", Kind: colScale, Anchor: "c2", Required: true, Width: 30},
		{Header: "C3", Group: groupEffort, Question: "C3. Complejidad técnica o procedural", Kind: colScale, Anchor: "c3", Required: true, Width: 30},
		{Header: "C4", Group: groupEffort, Question: "C4. Dependencias externas", Kind: colCoded, Coded: c4Coded, Required: true, Width: 16},

		{Header: "D1", Group: groupUrgency, Question: "D1. Fecha límite (deadline)", Kind: colDate, Width: 14,
			Help: "Déjala vacía si la actividad no tiene deadline."},
		{Header: "D2", Group: groupUrgency, Question: "D2. ¿Qué pasa si se retrasa 30 días?", Kind: colCoded, Coded: d2Coded, Required: true, Width: 26},
		{Header: "D3", Group: groupUrgency, Question: "D3. ¿Es importante para tus objetivos o para los de otra persona?", Kind: colCoded, Coded: d3Coded, Required: true, Width: 24},
		{Header: "D4", Group: groupUrgency, Question: "D4. ¿Un colega competente podría hacerlo con un briefing corto?", Kind: colCoded, Coded: d4Coded, Required: true, Width: 16},

		{Header: "F1", Group: groupInfo, Question: "F1. Confianza en las estimaciones anteriores", Kind: colScale, Anchor: "f1", Required: true, Width: 30},
		{Header: "F2", Group: groupInfo, Question: "F2. ¿Qué respuestas fueron conjeturas?", Kind: colF2, Width: 20,
			Help: "Códigos de pregunta separados por punto y coma, por ejemplo: B3;C1"},
		{Header: "F3", Group: groupInfo, Question: "F3. ¿Qué información falta y quién la tiene?", Kind: colText, Width: 40},
	}
}

// deliverableColumns son las columnas de la hoja Entregables.
func deliverableColumns() []xlsxColumn {
	return []xlsxColumn{
		{Header: "Actividad", Group: groupDlv, Question: "Actividad a la que pertenece el entregable", Kind: colText, Required: true, Width: 34,
			Help: "Debe coincidir con el Nombre de una fila de la hoja Actividades."},
		{Header: "Entregable", Group: groupDlv, Question: "Nombre del entregable", Kind: colText, Required: true, Width: 30},
		{Header: "Inicio", Group: groupDlv, Question: "Fecha de inicio (solo para el Gantt de este Excel)", Kind: colDate, Width: 14,
			Help: "Catchup guarda únicamente la fecha Fin; el Inicio dibuja la barra del Gantt. Si lo dejas vacío, la barra empieza en Fin."},
		{Header: "Fin", Group: groupDlv, Question: "Fecha del entregable", Kind: colDate, Required: true, Width: 14,
			Help: "Es la fecha que usa el timeline de Catchup."},
		{Header: "Estado", Group: groupDlv, Question: "Estado del entregable", Kind: colCoded, Coded: statusCoded, Width: 16,
			Help: "Si se deja vacío se importa como No iniciado."},
		{Header: "Depende de", Group: groupDlv, Question: "Entregable del que depende", Kind: colText, Width: 30,
			Help: "Nombre de otro entregable de la misma actividad. Se usa para propagar el riesgo en el timeline."},
	}
}
