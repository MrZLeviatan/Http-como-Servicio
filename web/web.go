// Package web contiene la interfaz visual (HTML, CSS y JS) y la incrusta
// dentro del binario con go:embed. Así el programa compilado funciona solo,
// sin necesidad de copiar la carpeta web/ junto a él.
package web

import "embed" // Permite incrustar archivos en el binario.

// Plantillas contiene los archivos HTML de templates/.
// La línea "//go:embed" (sin espacio tras //) le indica al compilador qué incluir.
//
//go:embed templates/*.html
var Plantillas embed.FS

// Estaticos contiene la carpeta static/ (CSS y JS).
//
//go:embed static
var Estaticos embed.FS
