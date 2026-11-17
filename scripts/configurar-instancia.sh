#!/bin/bash
# ↑ "Shebang": indica que este archivo se ejecuta con el intérprete bash.

# ============================================================================
# configurar-instancia.sh
# Configura una instancia recién creada a partir de la plantilla Apache.
# Se ejecuta DENTRO de la VM, con sudo.
#
# Uso:     configurar-instancia.sh <hostname> <ip>
# Ejemplo: configurar-instancia.sh paisajes 192.168.10.30
#
# Qué hace:
#   1. Cambia el nombre del equipo (hostname).
#   2. Crea la carpeta web /var/www/<hostname> con una página provisional.
#   3. Crea el VirtualHost de Apache para <hostname>.cloud.local.
#   4. Cambia la IP temporal de la plantilla (.20) por la IP definitiva.
#   5. Reinicia la VM para que todo se aplique.
# ============================================================================

# Modo estricto de bash:
#   -e  → si un comando falla, el script se detiene (no sigue a ciegas).
#   -u  → si se usa una variable que no existe, es un error.
#   -o pipefail → en "cmd1 | cmd2", si falla cmd1 también cuenta como fallo.
set -euo pipefail

# Dominio fijo del proyecto. Todas las instancias quedan como <nombre>.cloud.local
DOMINIO="cloud.local"

# $1 y $2 son el primer y segundo argumento que recibe el script.
# "${1:-}" significa: "el valor de $1, o vacío si no lo pasaron"
# (sin el ":-" el modo -u daría error antes de poder mostrar el mensaje de uso).
NOMBRE="${1:-}"
IP="${2:-}"

# --- Validaciones -----------------------------------------------------------

# -z "texto" es verdadero si el texto está vacío.
# Si falta el nombre O (||) falta la IP, muestra cómo se usa y termina.
if [[ -z "$NOMBRE" || -z "$IP" ]]; then
    # >&2 envía el mensaje a la salida de errores (stderr) en vez de la normal.
    echo "Uso: $0 <hostname> <ip>" >&2
    # exit 1 = terminar con código de error (0 sería "todo bien").
    exit 1
fi

# =~ compara contra una expresión regular.
# La regla: empieza y termina con letra minúscula o número, y en medio puede
# llevar guiones. Ej. válidos: "paisajes", "mi-sitio2". Inválidos: "Mi Sitio", "-x".
# El "!" niega: entra al if cuando el nombre NO cumple la regla.
if [[ ! "$NOMBRE" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]]; then
    echo "Hostname inválido: $NOMBRE (solo minúsculas, números y guiones)" >&2
    exit 1
fi

# La IP debe ser de la red del proyecto: 192.168.10.<1 a 3 dígitos>.
# \. es un punto literal; [0-9]{1,3} = entre 1 y 3 dígitos.
if [[ ! "$IP" =~ ^192\.168\.10\.[0-9]{1,3}$ ]]; then
    echo "IP inválida: $IP (debe ser 192.168.10.x)" >&2
    exit 1
fi

# --- 1. Hostname ------------------------------------------------------------

# Cambia el nombre del equipo de forma permanente (escribe /etc/hostname).
hostnamectl set-hostname "$NOMBRE"

# sed -i edita el archivo "en el sitio" (in-place).
# "s/A/B/" = sustituir A por B.
# Busca la línea que empieza (^) con 127.0.1.1 y la reemplaza completa (.*)
# por "127.0.1.1   paisajes.cloud.local paisajes".
# Así el propio equipo sabe resolver su nombre sin preguntar al DNS.
sed -i "s/^127\.0\.1\.1.*/127.0.1.1   $NOMBRE.$DOMINIO $NOMBRE/" /etc/hosts

# --- 2. Directorio web con una página provisional ---------------------------

# Carpeta donde vivirá la página de esta instancia, ej. /var/www/paisajes
DIR="/var/www/$NOMBRE"

# Crea la carpeta. -p = no da error si ya existe y crea carpetas intermedias.
mkdir -p "$DIR"

# "cat > archivo <<EOF ... EOF" escribe en el archivo todo el texto que hay
# entre las dos marcas EOF (esto se llama "heredoc").
# Las variables ($NOMBRE, $DOMINIO) se reemplazan por su valor.
# Esta página se ve hasta que el usuario publique su .zip.
cat > "$DIR/index.html" <<EOF
<!doctype html>
<html><head><meta charset="utf-8"><title>$NOMBRE</title></head>
<body>
  <h1>$NOMBRE.$DOMINIO</h1>
  <p>Instancia lista. Aún no se ha publicado contenido.</p>
</body></html>
EOF

# Hace dueño de la carpeta al usuario de Apache (www-data).
# -R = recursivo (la carpeta y todo lo que contiene).
chown -R www-data:www-data "$DIR"

# --- 3. VirtualHost de Apache -----------------------------------------------

# Un VirtualHost le dice a Apache: "cuando pidan ESTE nombre, sirve ESTA carpeta".
# Se guarda en sites-available/ (sitios disponibles), ej. paisajes.conf
#
# Dentro del bloque:
#   <VirtualHost *:80>  → escuchar en el puerto 80 (HTTP) en cualquier IP.
#   ServerName          → el nombre al que responde: paisajes.cloud.local
#   DocumentRoot        → la carpeta con los archivos de la página.
#   Options -Indexes    → si no hay index.html, NO mostrar la lista de archivos.
#           +FollowSymLinks → permitir enlaces simbólicos.
#   AllowOverride All   → permite que la página traiga su propio .htaccess.
#   Require all granted → cualquiera puede ver la página.
#   ErrorLog/CustomLog  → archivos de registro propios de esta instancia.
#
# \${APACHE_LOG_DIR} lleva "\" para que bash NO lo reemplace: debe quedar
# escrito tal cual en el archivo, porque es una variable que resuelve Apache.
cat > "/etc/apache2/sites-available/$NOMBRE.conf" <<EOF
<VirtualHost *:80>
    ServerName $NOMBRE.$DOMINIO
    DocumentRoot $DIR
    <Directory $DIR>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>
    ErrorLog \${APACHE_LOG_DIR}/$NOMBRE-error.log
    CustomLog \${APACHE_LOG_DIR}/$NOMBRE-access.log combined
</VirtualHost>
EOF

# Desactiva el sitio por defecto de Apache ("Apache2 Debian Default Page").
# -q = silencioso. "|| true" = si ya estaba desactivado y da error, ignorarlo
# (sin esto, "set -e" detendría el script).
a2dissite -q 000-default || true

# Activa el sitio nuevo (crea un enlace en sites-enabled/).
a2ensite -q "$NOMBRE"

# --- 4. IP definitiva -------------------------------------------------------

# Cambia la IP en /etc/network/interfaces.
#   -E = usar expresiones regulares "extendidas" (más legibles).
#   Se usa "|" como separador en lugar de "/" porque la IP lleva "/24".
#   ^([[:space:]]*address[[:space:]]+) → captura los espacios + la palabra
#       "address" + espacios, y la guarda como \1 para conservarla igual.
#   [0-9.]+/24 → la IP vieja (ej. 192.168.10.20/24).
#   \1$IP/24  → deja "    address " y pone la IP nueva.
# Solo la interfaz enp0s8 tiene línea "address", así que solo cambia esa.
sed -i -E "s|^([[:space:]]*address[[:space:]]+)[0-9.]+/24|\1$IP/24|" /etc/network/interfaces

# --- 5. Reinicio ------------------------------------------------------------

# Reinicia la VM para aplicar el hostname, la IP nueva y Apache.
#   sh -c '...'  → ejecuta esos comandos en una mini-terminal aparte.
#   sleep 2      → espera 2 segundos antes de reiniciar, para que este script
#                  alcance a terminar y responder "OK" por SSH.
#   nohup        → que siga vivo aunque se cierre la sesión SSH.
#   >/dev/null 2>&1 → descartar cualquier mensaje que produzca.
#   &            → ejecutarlo en segundo plano (no esperar a que termine).
nohup sh -c 'sleep 2; systemctl reboot' >/dev/null 2>&1 &

# Mensaje final. La app Go lo leerá para saber que todo salió bien.
# Después de esto la conexión SSH se corta (por el reinicio) y la VM vuelve
# con la IP nueva.
echo "OK: $NOMBRE.$DOMINIO -> $IP (reiniciando)"
