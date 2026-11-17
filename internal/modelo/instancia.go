// Package modelo define los datos centrales de la aplicación: qué es una
// instancia del servicio HTTP y en qué estados puede estar.
package modelo

import (
	"fmt"     // Formatear textos (URL, IP con máscara).
	"regexp"  // Expresiones regulares para validar nombres de host.
	"strings" // Limpiar y normalizar textos.
	"time"    // Fechas de creación y publicación.
)

// Estado representa la etapa del ciclo de vida en la que está una instancia.
// Es un string con nombre propio para que el compilador ayude a no mezclarlo
// con otros textos.
type Estado string

// Estados posibles de una instancia, en el orden normal en que ocurren.
const (
	EstadoCreando      Estado = "creando"      // Se está creando y arrancando la VM.
	EstadoConfigurando Estado = "configurando" // Se le está poniendo hostname, IP y Apache.
	EstadoLista        Estado = "lista"        // Funciona, pero aún sin contenido del usuario.
	EstadoPublicando   Estado = "publicando"   // Se está subiendo/descomprimiendo el .zip.
	EstadoPublicada    Estado = "publicada"    // La página del usuario ya está en línea.
	EstadoApagando     Estado = "apagando"     // Se está apagando la VM (sin borrarla).
	EstadoApagada      Estado = "apagada"      // La VM existe pero está apagada: la página no responde.
	EstadoEncendiendo  Estado = "encendiendo"  // Se está encendiendo la VM y esperando a que responda.
	EstadoEliminando   Estado = "eliminando"   // Se está apagando y borrando la VM.
	EstadoError        Estado = "error"        // Algo falló; ver el campo Detalle.
)

// Instancia es una máquina virtual con Apache que sirve la página de un usuario.
// Las etiquetas `json:"..."` definen cómo se guarda en data/instancias.json.
type Instancia struct {
	Nombre      string    `json:"nombre"`       // Hostname, ej. "paisajes".
	Dominio     string    `json:"dominio"`      // Nombre completo, ej. "paisajes.cloud.local".
	IP          string    `json:"ip"`           // IP asignada, ej. "192.168.10.30".
	Mascara     int       `json:"mascara"`      // Máscara de red, ej. 24.
	NombreVM    string    `json:"nombre_vm"`    // Nombre de la VM en VirtualBox.
	Estado      Estado    `json:"estado"`       // Etapa actual (ver constantes arriba).
	Detalle     string    `json:"detalle"`      // Último mensaje de progreso o de error.
	Archivo     string    `json:"archivo"`      // Nombre del último .zip publicado.
	CreadaEn    time.Time `json:"creada_en"`    // Cuándo se solicitó la instancia.
	PublicadaEn time.Time `json:"publicada_en"` // Cuándo se publicó contenido por última vez.
}

// URL devuelve el enlace para abrir la página, ej. "http://paisajes.cloud.local".
func (i Instancia) URL() string {
	return "http://" + i.Dominio
}

// IPConMascara devuelve la IP en formato CIDR, ej. "192.168.10.30/24".
func (i Instancia) IPConMascara() string {
	return fmt.Sprintf("%s/%d", i.IP, i.Mascara)
}

// EnProceso indica si la instancia está a mitad de una operación
// (en ese caso no se debe publicar ni eliminar hasta que termine).
func (i Instancia) EnProceso() bool {
	switch i.Estado {
	case EstadoCreando, EstadoConfigurando, EstadoPublicando,
		EstadoApagando, EstadoEncendiendo, EstadoEliminando:
		return true
	default:
		return false
	}
}

// Activa indica si la VM está encendida y sirviendo (lista o publicada).
func (i Instancia) Activa() bool {
	return i.Estado == EstadoLista || i.Estado == EstadoPublicada
}

// EstadoActivo devuelve el estado que le corresponde cuando está encendida:
// "publicada" si alguna vez se publicó contenido, o "lista" si no.
func (i Instancia) EstadoActivo() Estado {
	if i.PublicadaEn.IsZero() {
		return EstadoLista
	}
	return EstadoPublicada
}

// PuedePublicar indica si se le puede subir contenido (ya está funcionando).
func (i Instancia) PuedePublicar() bool {
	return i.Activa()
}

// PuedeApagar indica si se puede apagar (solo si está encendida y sin tareas).
func (i Instancia) PuedeApagar() bool {
	return i.Activa()
}

// PuedeEncender indica si se puede encender (solo si está apagada).
func (i Instancia) PuedeEncender() bool {
	return i.Estado == EstadoApagada
}

// PuedeEliminar indica si se puede borrar (cualquier estado que no esté a mitad de algo).
func (i Instancia) PuedeEliminar() bool {
	return !i.EnProceso()
}

// patronNombre: empieza y termina con letra minúscula o número, puede llevar
// guiones en medio y mide como máximo 63 caracteres (límite de DNS).
// Es la misma regla que valida scripts/configurar-instancia.sh.
var patronNombre = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizarNombre quita espacios a los lados y pasa el nombre a minúsculas.
func NormalizarNombre(nombre string) string {
	return strings.ToLower(strings.TrimSpace(nombre))
}

// ValidarNombre comprueba que el nombre sirva como hostname.
// Devuelve nil si es válido, o un error que explica el problema.
func ValidarNombre(nombre string) error {
	// El nombre no puede estar vacío.
	if nombre == "" {
		return fmt.Errorf("el nombre del host es obligatorio")
	}
	// Debe cumplir el patrón de hostname.
	if !patronNombre.MatchString(nombre) {
		return fmt.Errorf("nombre de host inválido '%s': usa solo minúsculas, números y guiones (sin espacios ni tildes), máximo 63 caracteres", nombre)
	}
	return nil
}
