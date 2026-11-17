// Package servidor implementa la aplicación web de gestión: el dashboard,
// los formularios (crear instancia y publicar contenido) y el botón eliminar.
//
// Sigue el patrón POST → Redirect → GET: tras cada formulario, el servidor
// redirige al dashboard con un mensaje. Así, recargar la página no repite
// la acción.
package servidor

import (
	"encoding/json" // Respuesta JSON del endpoint /api/instancias.
	"html/template" // Plantillas HTML con escape automático (evita inyección de código).
	"io/fs"         // Sub-sistemas de archivos embebidos.
	"log"           // Registrar errores en la terminal.
	"net/http"      // Servidor HTTP.
	"net/url"       // Armar la URL de redirección con el mensaje.
	"os"            // Hostname del PC.
	"strings"       // Comparar prefijos de rutas.
	"time"          // Formato de fechas.

	"httppaas/internal/almacen"        // Registro de instancias.
	"httppaas/internal/aprovisionador" // Orquestador.
	"httppaas/internal/config"         // Configuración.
	"httppaas/internal/modelo"         // Struct Instancia.
	"httppaas/web"                     // Plantillas y archivos estáticos embebidos.
)

// Servidor contiene lo que necesitan los handlers HTTP.
type Servidor struct {
	cfg        *config.Config                 // Configuración (zona, límites...).
	almacen    *almacen.Almacen               // Lectura del listado de instancias.
	aprov      *aprovisionador.Aprovisionador // Acciones: crear, publicar, eliminar.
	plantillas *template.Template             // Plantilla index.html ya cargada.
	equipo     string                         // Hostname del PC (se muestra en la cabecera).
}

// DatosPagina es todo lo que la plantilla index.html necesita para dibujarse.
type DatosPagina struct {
	Titulo      string             // Título de la página.
	Zona        string             // Dominio, ej. "cloud.local".
	Equipo      string             // Hostname del PC que ejecuta la app.
	Instancias  []modelo.Instancia // Todas las instancias (tabla).
	Publicables []modelo.Instancia // Solo las que aceptan contenido (lista desplegable).
	HayProcesos bool               // true si alguna está en curso (activa el refresco automático).
	Mensaje     string             // Mensaje de resultado de la última acción.
	TipoMensaje string             // "ok" o "error": define el color del mensaje.
	MaximoMB    int                // Tamaño máximo de .zip (para mostrarlo en el formulario).
}

// Nuevo carga la plantilla y crea el Servidor.
func Nuevo(cfg *config.Config, alm *almacen.Almacen, aprov *aprovisionador.Aprovisionador) (*Servidor, error) {
	// Funciones extra que la plantilla puede usar, ej. {{fecha .CreadaEn}}.
	funciones := template.FuncMap{
		"fecha": formatearFecha,
	}
	// Carga index.html desde los archivos embebidos en el binario.
	plantillas, err := template.New("").Funcs(funciones).ParseFS(web.Plantillas, "templates/*.html")
	if err != nil {
		return nil, err
	}
	// Hostname del PC; si falla, se muestra "desconocido".
	equipo, err := os.Hostname()
	if err != nil {
		equipo = "desconocido"
	}
	return &Servidor{cfg: cfg, almacen: alm, aprov: aprov, plantillas: plantillas, equipo: equipo}, nil
}

// Rutas registra cada URL con su handler y devuelve el enrutador.
// (Desde Go 1.22, el patrón puede incluir el método HTTP y variables {nombre}.)
func (s *Servidor) Rutas() http.Handler {
	mux := http.NewServeMux()

	// Página principal (dashboard). "{$}" = solo "/" exacto, no "/cualquier-cosa".
	mux.HandleFunc("GET /{$}", s.dashboard)
	// Formulario "Nombre del host" → botón Aceptar.
	mux.HandleFunc("POST /instancias", s.crearInstancia)
	// Formulario "Contenido web (comprimido)" → botón Publicar.
	mux.HandleFunc("POST /publicar", s.publicarContenido)
	// Botón Eliminar de cada fila.
	mux.HandleFunc("POST /instancias/{nombre}/eliminar", s.eliminarInstancia)
	// Botones Encender / Apagar de cada fila.
	mux.HandleFunc("POST /instancias/{nombre}/encender", s.encenderInstancia)
	mux.HandleFunc("POST /instancias/{nombre}/apagar", s.apagarInstancia)
	// Listado en JSON (lo consulta la página para saber cuándo refrescar).
	mux.HandleFunc("GET /api/instancias", s.apiInstancias)

	// Archivos estáticos (CSS, JS) embebidos en el binario, bajo /static/...
	estaticos, _ := fs.Sub(web.Estaticos, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(estaticos)))

	// Envuelve todo con el registro de peticiones.
	return registrarPeticiones(mux)
}

// dashboard dibuja la página principal con el listado de instancias.
func (s *Servidor) dashboard(w http.ResponseWriter, r *http.Request) {
	// Obtiene todas las instancias (copias seguras).
	instancias := s.almacen.Listar()

	// Prepara los datos de la plantilla.
	datos := DatosPagina{
		Titulo:      "Gestión de páginas y aplicaciones web",
		Zona:        s.cfg.DNS.Zona,
		Equipo:      s.equipo,
		Instancias:  instancias,
		Mensaje:     r.URL.Query().Get("msg"),  // Mensaje recibido en la URL (?msg=...).
		TipoMensaje: r.URL.Query().Get("tipo"), // Tipo recibido en la URL (?tipo=ok|error).
		MaximoMB:    s.cfg.Publicacion.TamanoMaximoMB,
	}
	// Recorre las instancias para llenar los campos derivados.
	for _, inst := range instancias {
		if inst.PuedePublicar() {
			datos.Publicables = append(datos.Publicables, inst)
		}
		if inst.EnProceso() {
			datos.HayProcesos = true
		}
	}

	// Indica que la respuesta es HTML en UTF-8.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Dibuja index.html con los datos.
	if err := s.plantillas.ExecuteTemplate(w, "index.html", datos); err != nil {
		log.Printf("[web] error al dibujar la plantilla: %v", err)
	}
}

// crearInstancia atiende el botón "Aceptar": pide una instancia nueva.
func (s *Servidor) crearInstancia(w http.ResponseWriter, r *http.Request) {
	// Lee el campo "nombre" del formulario.
	nombre := r.FormValue("nombre")
	// Pide la instancia al orquestador (responde rápido, el trabajo sigue en segundo plano).
	inst, err := s.aprov.SolicitarInstancia(nombre)
	if err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}
	redirigir(w, r, "ok", "Creando "+inst.Dominio+" con IP "+inst.IP+". El proceso tarda unos minutos.")
}

// publicarContenido atiende el botón "Publicar": recibe el .zip y lo envía a la instancia.
func (s *Servidor) publicarContenido(w http.ResponseWriter, r *http.Request) {
	// Limita el tamaño total de la petición; si se pasa, la lectura falla.
	// (+1 MB de margen para los demás campos del formulario.)
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.Publicacion.TamanoMaximoBytes()+(1<<20))

	// Lee el campo "nombre" (instancia elegida en la lista).
	nombre := r.FormValue("nombre")
	if nombre == "" {
		redirigir(w, r, "error", "Elige la instancia donde publicar")
		return
	}

	// Guarda el .zip subido en un archivo temporal del PC.
	rutaZip, nombreOriginal, err := guardarZip(r, "archivo")
	if err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}

	// Entrega el archivo al orquestador (él lo borra al terminar).
	if err := s.aprov.Publicar(nombre, rutaZip, nombreOriginal); err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}
	redirigir(w, r, "ok", "Publicando "+nombreOriginal+" en "+nombre+"."+s.cfg.DNS.Zona)
}

// eliminarInstancia atiende el botón "Eliminar" de una fila.
func (s *Servidor) eliminarInstancia(w http.ResponseWriter, r *http.Request) {
	// Lee la variable {nombre} de la URL /instancias/{nombre}/eliminar.
	nombre := r.PathValue("nombre")
	if err := s.aprov.Eliminar(nombre); err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}
	redirigir(w, r, "ok", "Eliminando "+nombre+"."+s.cfg.DNS.Zona)
}

// encenderInstancia atiende el botón "Encender" de una fila apagada.
func (s *Servidor) encenderInstancia(w http.ResponseWriter, r *http.Request) {
	// Lee la variable {nombre} de la URL /instancias/{nombre}/encender.
	nombre := r.PathValue("nombre")
	if err := s.aprov.Encender(nombre); err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}
	redirigir(w, r, "ok", "Encendiendo "+nombre+"."+s.cfg.DNS.Zona+". Tarda menos de un minuto.")
}

// apagarInstancia atiende el botón "Apagar" de una fila encendida.
func (s *Servidor) apagarInstancia(w http.ResponseWriter, r *http.Request) {
	// Lee la variable {nombre} de la URL /instancias/{nombre}/apagar.
	nombre := r.PathValue("nombre")
	if err := s.aprov.Apagar(nombre); err != nil {
		redirigir(w, r, "error", err.Error())
		return
	}
	redirigir(w, r, "ok", "Apagando "+nombre+"."+s.cfg.DNS.Zona+". Se conservan su contenido, su IP y su DNS.")
}

// apiInstancias devuelve el listado en JSON. La página lo consulta cada pocos
// segundos para refrescarse sola cuando cambia algún estado.
func (s *Servidor) apiInstancias(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Convierte el listado a JSON y lo escribe en la respuesta.
	if err := json.NewEncoder(w).Encode(s.almacen.Listar()); err != nil {
		log.Printf("[web] error al enviar JSON: %v", err)
	}
}

// redirigir envía al navegador de vuelta al dashboard con un mensaje.
// 303 See Other = "ve a esta otra página usando GET".
func redirigir(w http.ResponseWriter, r *http.Request, tipo, mensaje string) {
	// url.Values arma y codifica correctamente los parámetros ?tipo=...&msg=...
	parametros := url.Values{"tipo": {tipo}, "msg": {mensaje}}
	http.Redirect(w, r, "/?"+parametros.Encode(), http.StatusSeeOther)
}

// formatearFecha muestra una fecha como "2026-04-22 17:02:03" (o "—" si está vacía).
func formatearFecha(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	// En Go el formato se escribe con la fecha de referencia 2006-01-02 15:04:05.
	return t.Format("2006-01-02 15:04:05")
}

// registrarPeticiones es un "middleware": envuelve a otro handler para
// escribir en la terminal cada petición (método, ruta y duración).
func registrarPeticiones(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()      // Momento en que llegó la petición.
		siguiente.ServeHTTP(w, r) // Atiende la petición normalmente.
		if r.URL.Path == "/api/instancias" || strings.HasPrefix(r.URL.Path, "/static/") {
			return // No registrar consultas automáticas ni archivos estáticos (mucho ruido).
		}
		log.Printf("[web] %s %s (%v)", r.Method, r.URL.Path, time.Since(inicio).Round(time.Millisecond))
	})
}
