#!/bin/bash
# ↑ "Shebang": indica que este archivo se ejecuta con el intérprete bash.

# ============================================================================
# desplegar-sitio.sh
# Publica el contenido de un .zip en el sitio de la instancia.
# Se ejecuta DENTRO de la VM, con sudo, después de copiar el .zip por scp.
#
# Uso:     desplegar-sitio.sh <hostname> <archivo.zip>
# Ejemplo: desplegar-sitio.sh paisajes /tmp/sitio.zip
#
# Qué hace:
#   1. Descomprime el .zip en una carpeta temporal.
#   2. Si el .zip traía todo dentro de una sola carpeta, usa esa carpeta.
#   3. Reemplaza el contenido de /var/www/<hostname> por el nuevo.
#   4. Arregla dueño y permisos para que Apache pueda leerlo.
#   5. Borra el .zip y recarga Apache.
# ============================================================================

# Modo estricto de bash:
#   -e  → si un comando falla, el script se detiene.
#   -u  → usar una variable que no existe es un error.
#   -o pipefail → en "cmd1 | cmd2", si falla cmd1 también cuenta como fallo.
set -euo pipefail

# $1 = nombre de la instancia, $2 = ruta del .zip.
# "${1:-}" = el valor de $1, o vacío si no lo pasaron.
NOMBRE="${1:-}"
ZIP="${2:-}"

# --- Validaciones -----------------------------------------------------------

# Si falta alguno de los dos argumentos, muestra cómo se usa y termina con error.
if [[ -z "$NOMBRE" || -z "$ZIP" ]]; then
    echo "Uso: $0 <hostname> <archivo.zip>" >&2
    exit 1
fi

# -f "ruta" es verdadero si existe y es un archivo normal.
# Si el .zip no existe (ej. falló el scp), termina con error.
if [[ ! -f "$ZIP" ]]; then
    echo "No existe el archivo: $ZIP" >&2
    exit 1
fi

# --- 1. Descomprimir en una carpeta temporal --------------------------------

# Carpeta final de la página, ej. /var/www/paisajes
DIR="/var/www/$NOMBRE"

# mktemp -d crea una carpeta temporal con nombre único (ej. /tmp/tmp.Ab12Cd)
# y devuelve su ruta. $( ... ) guarda esa ruta en la variable TMP.
TMP="$(mktemp -d)"

# trap '...' EXIT = "cuando el script termine (bien o mal), ejecuta esto".
# Así la carpeta temporal siempre se borra, aunque algo falle a mitad.
trap 'rm -rf "$TMP"' EXIT

# Descomprime el .zip dentro de la carpeta temporal.
#   -q = silencioso (no lista cada archivo).
#   -d = carpeta de destino.
# Se descomprime primero aparte para no dejar la página a medias si el zip
# está dañado.
unzip -q "$ZIP" -d "$TMP"

# Los .zip creados en Mac traen una carpeta basura "__MACOSX". Se elimina.
# -f = no da error si no existe.
rm -rf "$TMP/__MACOSX"

# --- 2. ¿El zip traía todo dentro de una carpeta? ---------------------------

# Es común comprimir la carpeta entera, ej. "mi-sitio/index.html", en vez de
# solo su contenido. En ese caso hay que usar lo que está DENTRO de "mi-sitio".

# Por defecto, el origen es la raíz de lo descomprimido.
ORIGEN="$TMP"

# find ... -mindepth 1 -maxdepth 1 → lista solo lo que hay en el primer nivel
#                                    de la carpeta temporal (sin entrar más).
# mapfile -t ENTRADAS < <(...)     → guarda cada línea de esa lista como un
#                                    elemento del arreglo ENTRADAS.
mapfile -t ENTRADAS < <(find "$TMP" -mindepth 1 -maxdepth 1)

# ${#ENTRADAS[@]} = cuántos elementos hay en el primer nivel.
# -d = es una carpeta.
# Si hay exactamente UNA cosa y es una carpeta, el origen pasa a ser esa carpeta.
if [[ ${#ENTRADAS[@]} -eq 1 && -d "${ENTRADAS[0]}" ]]; then
    ORIGEN="${ENTRADAS[0]}"
fi

# --- 3. Reemplazar el contenido anterior ------------------------------------

# Borra la carpeta de la página (incluida la página provisional o una versión
# anterior) para que no queden archivos viejos mezclados.
rm -rf "$DIR"

# La vuelve a crear vacía.
mkdir -p "$DIR"

# Copia todo el contenido del origen a la carpeta de la página.
#   -a = conserva subcarpetas, fechas y estructura.
#   "$ORIGEN"/. = "el contenido de ORIGEN", incluidos archivos ocultos
#                 como .htaccess (con /* se perderían).
cp -a "$ORIGEN"/. "$DIR"/

# --- 4. Dueño y permisos ----------------------------------------------------

# Hace dueño de todo al usuario de Apache (www-data). -R = recursivo.
chown -R www-data:www-data "$DIR"

# Permisos de carpetas: 755 = el dueño puede todo; los demás pueden entrar y leer.
#   find "$DIR" -type d → busca todas las carpetas.
#   -exec chmod 755 {} + → les aplica chmod 755 (los {} se reemplazan por las rutas).
find "$DIR" -type d -exec chmod 755 {} +

# Permisos de archivos: 644 = el dueño lee y escribe; los demás solo leen.
# Así ningún archivo subido queda como ejecutable.
find "$DIR" -type f -exec chmod 644 {} +

# --- 5. Limpieza y recarga ---------------------------------------------------

# Borra el .zip original, que ya no se necesita.
rm -f "$ZIP"

# Recarga Apache para que tome los cambios sin cortar las conexiones abiertas
# (reload es más suave que restart).
systemctl reload apache2

# Mensaje final. La app Go lo leerá para saber que todo salió bien.
echo "OK: contenido publicado en http://$NOMBRE.cloud.local"
