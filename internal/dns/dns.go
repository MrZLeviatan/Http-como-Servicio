// Package dns registra y elimina nombres en la zona de Bind9 (cloud.local)
// usando nsupdate y la llave TSIG, sin editar archivos de zona a mano
// (actualización dinámica de DNS, RFC 2136).
package dns

import (
	"context" // Tiempo límite y cancelación.
	"fmt"     // Formatear las órdenes y los errores.
	"net"     // Hacer consultas DNS para verificar.
	"slices"  // Buscar un valor dentro de una lista.
	"strconv" // Convertir números a texto.
	"strings" // Construir el texto de órdenes para nsupdate.
	"time"    // Tiempo límite de conexión.

	"httppaas/internal/config"   // Configuración DNS.
	"httppaas/internal/ejecutor" // Ejecutar nsupdate.
)

// Cliente actualiza el servidor DNS configurado.
type Cliente struct {
	cfg config.DNS // Sección "dns" de config.json.
}

// Nuevo crea un Cliente DNS.
func Nuevo(cfg config.DNS) *Cliente {
	return &Cliente{cfg: cfg}
}

// Zona devuelve la zona administrada, ej. "cloud.local".
func (c *Cliente) Zona() string {
	return c.cfg.Zona
}

// NombreCompleto devuelve "nombre.zona", ej. "paisajes.cloud.local".
func (c *Cliente) NombreCompleto(nombre string) string {
	return nombre + "." + c.cfg.Zona
}

// EsReservado indica si el nombre está en la lista de hosts reservados
// (ej. "ns", que es el propio servidor DNS).
func (c *Cliente) EsReservado(nombre string) bool {
	return slices.Contains(c.cfg.HostsReservados, nombre)
}

// Registrar crea (o reemplaza) el registro A: nombre.zona → ip.
func (c *Cliente) Registrar(ctx context.Context, nombre, ip string) error {
	fqdn := c.NombreCompleto(nombre) // Ej. paisajes.cloud.local
	return c.enviar(ctx,
		// Primero borra cualquier registro A anterior con ese nombre...
		fmt.Sprintf("update delete %s. A", fqdn),
		// ...y luego agrega el nuevo: nombre, TTL, tipo A, IP.
		fmt.Sprintf("update add %s. %d A %s", fqdn, c.cfg.TTL, ip),
	)
}

// Eliminar borra el registro A de nombre.zona (si no existe, no pasa nada).
func (c *Cliente) Eliminar(ctx context.Context, nombre string) error {
	return c.enviar(ctx, fmt.Sprintf("update delete %s. A", c.NombreCompleto(nombre)))
}

// enviar ejecuta nsupdate con las órdenes indicadas.
//
// nsupdate lee sus órdenes por la entrada estándar, igual que en la terminal:
//
//	server 192.168.10.10
//	zone cloud.local
//	update ...
//	send
func (c *Cliente) enviar(ctx context.Context, ordenes ...string) error {
	// Construye el texto completo línea por línea.
	var texto strings.Builder
	texto.WriteString("server " + c.cfg.Servidor + "\n") // A qué servidor DNS hablarle.
	texto.WriteString("zone " + c.cfg.Zona + "\n")       // Qué zona se modifica.
	for _, orden := range ordenes {                      // Las órdenes de cambio.
		texto.WriteString(orden + "\n")
	}
	texto.WriteString("send\n") // Enviar todos los cambios juntos.

	// Ejecuta: nsupdate -k <llave> -t 10   (con el texto como entrada)
	//   -k: archivo con la llave TSIG que autoriza los cambios.
	//   -t: segundos máximos de espera por la respuesta del servidor.
	_, err := ejecutor.Ejecutar(ctx, strings.NewReader(texto.String()),
		c.cfg.Ejecutable, "-k", c.cfg.LlaveTSIG, "-t", "10")
	if err != nil {
		return fmt.Errorf("no se pudo actualizar el DNS: %w", err)
	}
	return nil
}

// Resolver pregunta DIRECTAMENTE al servidor DNS configurado qué IP tiene
// nombre.zona. Sirve para verificar que el registro quedó bien, sin depender
// de la configuración DNS del PC.
func (c *Cliente) Resolver(ctx context.Context, nombre string) ([]string, error) {
	// Un Resolver de Go que, en vez del DNS del sistema, usa siempre Bind9.
	resolvedor := &net.Resolver{
		PreferGo: true, // Usar la implementación de Go (permite cambiar el servidor).
		Dial: func(ctx context.Context, red, _ string) (net.Conn, error) {
			// Ignora la dirección por defecto y se conecta al puerto 53 de Bind9.
			marcador := net.Dialer{Timeout: 3 * time.Second}
			return marcador.DialContext(ctx, red, net.JoinHostPort(c.cfg.Servidor, strconv.Itoa(53)))
		},
	}
	// Hace la consulta y devuelve las IPs encontradas.
	return resolvedor.LookupHost(ctx, c.NombreCompleto(nombre))
}
