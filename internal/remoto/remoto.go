// Package remoto ejecuta comandos y copia archivos en las VMs usando los
// programas ssh y scp del sistema, con la llave privada configurada (sin
// contraseña).
package remoto

import (
	"context" // Tiempo límite y cancelación.
	"fmt"     // Formatear argumentos y errores.
	"strconv" // Convertir números a texto.
	"strings" // Armar comandos de forma segura.
	"time"    // Pausas y plazos de espera.

	"httppaas/internal/config"   // Configuración SSH.
	"httppaas/internal/ejecutor" // Ejecutar ssh y scp.
)

// CodigoErrorConexion es el código con el que ssh termina cuando falla la
// conexión (y no el comando remoto). Ej.: la VM se reinició a mitad.
const CodigoErrorConexion = 255

// Cliente ejecuta ssh/scp con las opciones del proyecto.
type Cliente struct {
	cfg config.SSH // Sección "ssh" de config.json.
}

// Nuevo crea un Cliente SSH.
func Nuevo(cfg config.SSH) *Cliente {
	return &Cliente{cfg: cfg}
}

// opciones devuelve los argumentos comunes de ssh y scp.
func (c *Cliente) opciones() []string {
	return []string{
		"-i", c.cfg.LlavePrivada, // Usar esta llave privada.
		"-o", "IdentitiesOnly=yes", // Probar SOLO esa llave (evita "Too many authentication failures").
		"-o", "BatchMode=yes", // Nunca preguntar nada (contraseñas, confirmaciones): falla en su lugar.
		// Todas las instancias salen del mismo disco, así que comparten la misma
		// "huella" SSH, y las IPs se reutilizan. Para este laboratorio no se
		// guardan ni verifican huellas; en producción NO se debe hacer esto.
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR", // No mostrar advertencias como "Permanently added...".
		"-o", "ConnectTimeout=" + strconv.Itoa(c.cfg.TiempoConexionSeg), // Máx. segundos para conectar.
		"-o", "ServerAliveInterval=5", // Cada 5 s comprobar que la VM sigue respondiendo...
		"-o", "ServerAliveCountMax=2", // ...y cortar tras 2 fallos (ej. si la VM se reinicia).
	}
}

// destino arma "usuario@ip".
func (c *Cliente) destino(ip string) string {
	return c.cfg.Usuario + "@" + ip
}

// Ejecutar corre `comando` dentro de la VM con IP `ip` y devuelve su salida.
func (c *Cliente) Ejecutar(ctx context.Context, ip, comando string) (string, error) {
	// Argumentos finales: opciones + usuario@ip + comando.
	args := append(c.opciones(), c.destino(ip), comando)
	return ejecutor.Ejecutar(ctx, nil, "ssh", args...)
}

// Copiar sube el archivo local `origen` a la ruta `destino` dentro de la VM.
func (c *Cliente) Copiar(ctx context.Context, origen, ip, destino string) error {
	// scp usa la sintaxis: scp <opciones> archivo_local usuario@ip:ruta_remota
	args := append(c.opciones(), origen, c.destino(ip)+":"+destino)
	_, err := ejecutor.Ejecutar(ctx, nil, "scp", args...)
	return err
}

// Responde hace UN intento rápido de conexión a `ip`. Devuelve true si entra.
func (c *Cliente) Responde(ctx context.Context, ip string) bool {
	// "true" es un comando que no hace nada y siempre termina bien.
	_, err := c.Ejecutar(ctx, ip, "true")
	return err == nil
}

// EsperarDisponible reintenta conectarse a `ip` cada 3 segundos hasta que
// responda, o hasta que pase el tiempo de espera de arranque configurado.
func (c *Cliente) EsperarDisponible(ctx context.Context, ip string) error {
	// Momento límite para dejar de intentar.
	limite := time.Now().Add(time.Duration(c.cfg.EsperaArranqueSeg) * time.Second)
	for {
		// Si responde, la VM ya arrancó y SSH está listo.
		if c.Responde(ctx, ip) {
			return nil
		}
		// Si se pasó el tiempo, se rinde.
		if time.Now().After(limite) {
			return fmt.Errorf("la VM en %s no respondió por SSH después de %d segundos", ip, c.cfg.EsperaArranqueSeg)
		}
		// Espera 3 segundos o hasta que se cancele el contexto.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// Comando arma una línea de comando segura para la terminal remota, poniendo
// cada argumento entre comillas simples. Así un nombre con caracteres raros
// nunca se interpreta como otro comando.
// Ej.: Comando("sudo", "script.sh", "paisajes") → 'sudo' 'script.sh' 'paisajes'
func Comando(partes ...string) string {
	citadas := make([]string, len(partes))
	for i, parte := range partes {
		// Dentro de comillas simples, la única forma de escribir ' es: '\''
		citadas[i] = "'" + strings.ReplaceAll(parte, "'", `'\''`) + "'"
	}
	return strings.Join(citadas, " ")
}
