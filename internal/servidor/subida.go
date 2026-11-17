package servidor

import (
	"bytes"         // Comparar los primeros bytes del archivo.
	"errors"        // Detectar el error de tamaño máximo.
	"fmt"           // Formatear mensajes de error.
	"io"            // Copiar datos entre lector y archivo.
	"net/http"      // Petición HTTP y error de tamaño.
	"os"            // Crear el archivo temporal.
	"path/filepath" // Extensión y nombre del archivo.
	"strings"       // Pasar la extensión a minúsculas.
)

// firmaZip son los 4 primeros bytes de todo archivo .zip ("PK\x03\x04").
// Revisarlos evita aceptar un archivo cualquiera renombrado a .zip.
var firmaZip = []byte{'P', 'K', 0x03, 0x04}

// guardarZip lee el archivo enviado en el campo `campo` del formulario,
// comprueba que sea un .zip real y lo guarda en un archivo temporal.
// Devuelve la ruta temporal y el nombre original del archivo.
func guardarZip(r *http.Request, campo string) (ruta, nombreOriginal string, err error) {
	// Obtiene el archivo subido y su cabecera (nombre, tamaño...).
	archivo, cabecera, err := r.FormFile(campo)
	if err != nil {
		// Si se pasó del tamaño máximo, MaxBytesReader produce este error.
		var demasiadoGrande *http.MaxBytesError
		if errors.As(err, &demasiadoGrande) {
			return "", "", fmt.Errorf("el archivo supera el tamaño máximo permitido")
		}
		return "", "", fmt.Errorf("selecciona un archivo .zip para publicar")
	}
	// Cierra el archivo subido al terminar.
	defer archivo.Close()

	// Nombre original sin rutas (algunos navegadores envían la ruta completa).
	nombreOriginal = filepath.Base(cabecera.Filename)

	// Comprueba la extensión.
	if strings.ToLower(filepath.Ext(nombreOriginal)) != ".zip" {
		return "", "", fmt.Errorf("'%s' no es un archivo .zip", nombreOriginal)
	}

	// Lee los primeros 4 bytes para comprobar la firma ZIP.
	inicio := make([]byte, len(firmaZip))
	if _, err := io.ReadFull(archivo, inicio); err != nil || !bytes.Equal(inicio, firmaZip) {
		return "", "", fmt.Errorf("'%s' no es un .zip válido o está vacío", nombreOriginal)
	}

	// Crea un archivo temporal único, ej. /tmp/httppaas-123456.zip
	temporal, err := os.CreateTemp("", "httppaas-*.zip")
	if err != nil {
		return "", "", fmt.Errorf("no se pudo guardar el archivo: %w", err)
	}
	defer temporal.Close()

	// Escribe primero los 4 bytes ya leídos y luego el resto del archivo.
	// io.MultiReader los encadena como si fueran un solo lector.
	if _, err := io.Copy(temporal, io.MultiReader(bytes.NewReader(inicio), archivo)); err != nil {
		// Si falla a mitad, borra el archivo incompleto.
		_ = os.Remove(temporal.Name())
		return "", "", fmt.Errorf("no se pudo guardar el archivo: %w", err)
	}

	// Devuelve la ruta del temporal y el nombre original.
	return temporal.Name(), nombreOriginal, nil
}
