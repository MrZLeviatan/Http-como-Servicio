// Package almacen guarda el listado de instancias en un archivo JSON para que
// no se pierda al cerrar la aplicación.
//
// Varias goroutines (la web y los procesos en segundo plano) leen y escriben
// al mismo tiempo, por eso todo acceso pasa por un candado (sync.Mutex).
package almacen

import (
	"encoding/json" // Convertir instancias a JSON y de vuelta.
	"errors"        // Comparar errores (archivo no existe).
	"fmt"           // Formatear mensajes de error.
	"io/fs"         // Error estándar "archivo no existe".
	"os"            // Leer, escribir y renombrar archivos.
	"path/filepath" // Obtener la carpeta de un archivo.
	"sort"          // Ordenar el listado por fecha.
	"sync"          // Candado para acceso concurrente.

	"httppaas/internal/modelo" // Struct Instancia y sus estados.
)

// ErrNoExiste se devuelve cuando se busca una instancia que no está registrada.
var ErrNoExiste = errors.New("la instancia no existe")

// Almacen mantiene las instancias en memoria y las copia al disco en cada cambio.
type Almacen struct {
	ruta       string                       // Ruta del archivo JSON.
	mu         sync.Mutex                   // Candado: solo una goroutine a la vez toca el mapa.
	instancias map[string]*modelo.Instancia // Instancias indexadas por nombre.
}

// Abrir carga el archivo `ruta` (si existe) y devuelve el almacén listo.
// Si el archivo no existe, empieza vacío y lo creará al primer cambio.
func Abrir(ruta string) (*Almacen, error) {
	// Crea la carpeta del archivo (ej. "data/") si no existe. 0o755 = permisos rwxr-xr-x.
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return nil, fmt.Errorf("no se pudo crear la carpeta de datos: %w", err)
	}

	// Crea el almacén con el mapa vacío.
	a := &Almacen{ruta: ruta, instancias: make(map[string]*modelo.Instancia)}

	// Lee el contenido del archivo.
	datos, err := os.ReadFile(ruta)
	// Si no existe, no es un error: es la primera vez que se ejecuta la app.
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer '%s': %w", ruta, err)
	}

	// Convierte el JSON (una lista) en un slice de instancias.
	var lista []*modelo.Instancia
	if err := json.Unmarshal(datos, &lista); err != nil {
		return nil, fmt.Errorf("'%s' está dañado: %w", ruta, err)
	}

	// Indica si hubo que corregir algo al cargar.
	corregido := false
	// Pasa cada instancia de la lista al mapa.
	for _, inst := range lista {
		// Si la app se cerró a mitad de una operación, esa operación ya no
		// sigue corriendo: se marca como error para que el usuario lo sepa.
		if inst.EnProceso() {
			inst.Estado = modelo.EstadoError
			inst.Detalle = "La operación se interrumpió porque la aplicación se cerró. Puedes eliminar la instancia."
			corregido = true
		}
		a.instancias[inst.Nombre] = inst
	}

	// Si se corrigió algo, se guarda de inmediato.
	if corregido {
		if err := a.guardarSinCandado(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// Listar devuelve una COPIA de todas las instancias, de la más antigua a la más nueva.
// Se devuelven copias para que nadie modifique el mapa sin pasar por el candado.
func (a *Almacen) Listar() []modelo.Instancia {
	// Toma el candado y lo suelta al terminar la función.
	a.mu.Lock()
	defer a.mu.Unlock()

	// Prepara un slice con espacio para todas las instancias.
	lista := make([]modelo.Instancia, 0, len(a.instancias))
	// Copia cada instancia (el * copia el valor, no el puntero).
	for _, inst := range a.instancias {
		lista = append(lista, *inst)
	}
	// Ordena por fecha de creación (las más antiguas primero).
	sort.Slice(lista, func(i, j int) bool {
		return lista[i].CreadaEn.Before(lista[j].CreadaEn)
	})
	return lista
}

// Obtener devuelve una copia de la instancia `nombre`, o ErrNoExiste.
func (a *Almacen) Obtener(nombre string) (modelo.Instancia, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Busca en el mapa; "ok" es false si no está.
	inst, ok := a.instancias[nombre]
	if !ok {
		return modelo.Instancia{}, ErrNoExiste
	}
	return *inst, nil
}

// Agregar registra una instancia nueva. Falla si ya existe una con ese nombre.
func (a *Almacen) Agregar(inst modelo.Instancia) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// No se permiten dos instancias con el mismo nombre.
	if _, existe := a.instancias[inst.Nombre]; existe {
		return fmt.Errorf("ya existe una instancia llamada '%s'", inst.Nombre)
	}
	// Guarda un puntero a una copia propia de la instancia.
	copia := inst
	a.instancias[inst.Nombre] = &copia
	// Escribe el cambio en disco.
	return a.guardarSinCandado()
}

// Actualizar modifica la instancia `nombre` aplicando la función `cambio`.
//
// `cambio` se ejecuta CON el candado tomado, así que puede revisar el estado
// actual y decidir de forma segura. Si `cambio` devuelve un error, no se
// guarda nada y ese error se devuelve.
// Devuelve la instancia como quedó después del cambio.
func (a *Almacen) Actualizar(nombre string, cambio func(*modelo.Instancia) error) (modelo.Instancia, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Busca la instancia.
	inst, ok := a.instancias[nombre]
	if !ok {
		return modelo.Instancia{}, ErrNoExiste
	}
	// Trabaja sobre una copia: si el cambio falla, el original queda intacto.
	copia := *inst
	if err := cambio(&copia); err != nil {
		return *inst, err
	}
	// El cambio fue aceptado: reemplaza la instancia y guarda en disco.
	*inst = copia
	return copia, a.guardarSinCandado()
}

// Eliminar quita la instancia `nombre` del registro (no toca la VM).
func (a *Almacen) Eliminar(nombre string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// delete no falla si la clave no existe.
	delete(a.instancias, nombre)
	return a.guardarSinCandado()
}

// guardarSinCandado escribe todas las instancias en el archivo JSON.
// Debe llamarse con el candado YA tomado (de ahí su nombre).
//
// Para no dañar el archivo si el programa se cierra a mitad de la escritura,
// primero escribe un archivo temporal y luego lo renombra (operación atómica).
func (a *Almacen) guardarSinCandado() error {
	// Pasa el mapa a una lista ordenada por fecha.
	lista := make([]*modelo.Instancia, 0, len(a.instancias))
	for _, inst := range a.instancias {
		lista = append(lista, inst)
	}
	sort.Slice(lista, func(i, j int) bool {
		return lista[i].CreadaEn.Before(lista[j].CreadaEn)
	})

	// Convierte la lista a JSON con sangría de 2 espacios (legible para humanos).
	datos, err := json.MarshalIndent(lista, "", "  ")
	if err != nil {
		return fmt.Errorf("no se pudo convertir a JSON: %w", err)
	}

	// Escribe el archivo temporal. 0o644 = rw-r--r--
	temporal := a.ruta + ".tmp"
	if err := os.WriteFile(temporal, datos, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir '%s': %w", temporal, err)
	}
	// Reemplaza el archivo real por el temporal de una sola vez.
	if err := os.Rename(temporal, a.ruta); err != nil {
		return fmt.Errorf("no se pudo guardar '%s': %w", a.ruta, err)
	}
	return nil
}
