// Package ejecutor centraliza cómo la aplicación lanza programas externos
// (VBoxManage, ssh, scp, nsupdate) y captura lo que responden.
//
// Tenerlo en un solo lugar garantiza que todos los errores incluyan el
// comando ejecutado y el mensaje que imprimió, lo que facilita depurar.
package ejecutor

import (
	"bytes"   // Buffers en memoria para guardar la salida del programa.
	"context" // Permite cancelar el programa o ponerle tiempo límite.
	"errors"  // Inspeccionar el tipo de error (código de salida).
	"fmt"     // Formatear el mensaje de error.
	"io"      // Tipo io.Reader para la entrada estándar.
	"os/exec" // Ejecutar programas del sistema.
	"strings" // Unir argumentos y limpiar espacios.
)

// Ejecutar lanza el programa `nombre` con sus `args` y espera a que termine.
//
//   - ctx: si se cancela o vence su tiempo, el programa se detiene.
//   - entrada: texto que se le envía por su entrada estándar (stdin); puede ser nil.
//
// Devuelve lo que el programa imprimió en su salida estándar (stdout).
// Si el programa termina con error, el error incluye lo que imprimió en stderr.
func Ejecutar(ctx context.Context, entrada io.Reader, nombre string, args ...string) (string, error) {
	// Prepara el comando (todavía no lo ejecuta).
	cmd := exec.CommandContext(ctx, nombre, args...)
	// Conecta la entrada estándar (si es nil, el programa no recibe nada).
	cmd.Stdin = entrada

	// Buffers donde se guardará lo que el programa imprima.
	var salida, errores bytes.Buffer
	cmd.Stdout = &salida  // Salida normal.
	cmd.Stderr = &errores // Mensajes de error.

	// Ejecuta el programa y espera a que termine.
	if err := cmd.Run(); err != nil {
		// Prefiere el mensaje de stderr; si está vacío, usa stdout.
		detalle := strings.TrimSpace(errores.String())
		if detalle == "" {
			detalle = strings.TrimSpace(salida.String())
		}
		// Arma un error con: programa + argumentos + error original + mensaje.
		// %w "envuelve" el error original para poder inspeccionarlo después.
		return strings.TrimSpace(salida.String()), fmt.Errorf("%s %s: %w: %s",
			nombre, strings.Join(args, " "), err, detalle)
	}

	// Todo bien: devuelve la salida sin espacios ni saltos de línea sobrantes.
	return strings.TrimSpace(salida.String()), nil
}

// CodigoSalida extrae el código de salida de un error devuelto por Ejecutar.
// Devuelve -1 si el error no proviene de un programa que terminó con código.
// (Ejemplo: ssh devuelve 255 cuando falla la CONEXIÓN, y otro número cuando
// falla el COMANDO remoto.)
func CodigoSalida(err error) int {
	// errors.As busca dentro del error envuelto un *exec.ExitError.
	var salida *exec.ExitError
	if errors.As(err, &salida) {
		return salida.ExitCode()
	}
	return -1
}
