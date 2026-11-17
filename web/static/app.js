// app.js — comportamiento de la página de gestión.
//  1. Pide confirmación antes de eliminar una instancia.
//  2. Recarga la página sola cuando cambia el estado de alguna instancia
//     (ej. pasa de "creando" a "lista"), sin borrar lo que el usuario esté
//     escribiendo en los formularios.

(function () {
    "use strict";

    // ── 1. Confirmación antes de eliminar ──────────────────────────────
    // Cada formulario con data-confirmar muestra ese texto antes de enviarse.
    document.querySelectorAll("form[data-confirmar]").forEach(function (form) {
        form.addEventListener("submit", function (evento) {
            // Si el usuario pulsa "Cancelar", se detiene el envío.
            if (!window.confirm(form.dataset.confirmar)) {
                evento.preventDefault();
            }
        });
    });

    // ── 2. Refresco automático ─────────────────────────────────────────

    // Resume el listado en un texto "nombre:estado:detalle|..." para comparar rápido.
    function firma(instancias) {
        return instancias
            .map(function (i) { return i.nombre + ":" + i.estado + ":" + i.detalle; })
            .join("|");
    }

    // Indica si el usuario está usando algún formulario (escribió un nombre o
    // eligió un archivo). En ese caso no se recarga para no borrar su trabajo.
    function formularioEnUso() {
        var nombre = document.getElementById("nombre");
        var archivo = document.getElementById("archivo");
        return (nombre && nombre.value.trim() !== "") ||
               (archivo && archivo.files && archivo.files.length > 0) ||
               (document.activeElement && document.activeElement.closest("[data-formulario]"));
    }

    // Firma del estado que se dibujó al cargar la página (se obtiene en la 1.ª consulta).
    var firmaInicial = null;

    // Consulta /api/instancias y recarga si algo cambió.
    function revisar() {
        fetch("/api/instancias", { cache: "no-store" })
            .then(function (respuesta) { return respuesta.json(); })
            .then(function (instancias) {
                var actual = firma(instancias || []);
                if (firmaInicial === null) {
                    firmaInicial = actual;           // Primera consulta: solo memorizar.
                } else if (actual !== firmaInicial && !formularioEnUso()) {
                    // Algo cambió: recargar SIN los parámetros ?msg=... para no repetir el mensaje.
                    window.location.replace("/");
                }
            })
            .catch(function () { /* Si el servidor no responde, se reintenta en la próxima vuelta. */ });
    }

    // Consulta enseguida y luego cada 4 segundos.
    revisar();
    window.setInterval(revisar, 4000);
})();
