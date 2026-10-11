const $ = (sel) => document.querySelector(sel);
const esc = (s) => String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

const SECCIONES = {
  arranque: ["Arranque", "Se aplican los manifiestos y se espera a que los cuatro Deployments estén listos."],
  pedido: ["Un pedido completo", "Todo pasa por el ESB: el cliente pide, el restaurante prepara, el repartidor entrega."],
  "caos-esb": ["Caos: se elimina el pod del ESB",
    "Mientras una sonda hace una petición real cada 100 ms, se borra el pod del bus. Kubernetes crea otro, vacío, y los servicios se vuelven a registrar."],
  "caos-restaurante": ["Caos: se elimina el pod del restaurante",
    "La misma sonda, ahora borrando el pod que recibe los pedidos."],
};

function segundos(ms) {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${ms} ms`;
}

function sonda(m, idFallas, idSub) {
  if (!m) return;
  $(idFallas).textContent = `${m.fallidas} de ${m.peticiones}`;
  $(idSub).textContent = `peticiones fallaron · la más lenta tardó ${segundos(m.latencia_max_ms)}`;
}

fetch("reporte.json", { cache: "no-cache" })
  .then((r) => r.json())
  .then((rep) => {
    const total = rep.pasos.length;
    const ok = rep.pasos.filter((p) => p.ok).length;
    const fecha = new Date(rep.generado).toLocaleString("es-GT", { dateStyle: "medium", timeStyle: "short" });
    $("#resumen").classList.add(ok === total ? "ok" : "bad");
    $("#resumenTexto").textContent =
      `${ok === total ? "✓" : "✗"} ${ok} de ${total} pasos · Kubernetes ${rep.kubernetes} · ${Math.round(rep.duracion_s)} s · ${fecha}`;
    if (rep.run_url) $("#runLink").href = rep.run_url;

    const m = rep.metricas;
    sonda(m.sonda_esb, "#esbFallas", "#esbSub");
    sonda(m.sonda_restaurante, "#resFallas", "#resSub");
    if (m.recuperacion_esb_s != null) $("#reaprende").textContent = `${m.recuperacion_esb_s} s`;

    const grupos = {};
    for (const p of rep.pasos) (grupos[p.seccion] ||= []).push(p);
    $("#pasos").innerHTML = Object.entries(grupos).map(([sec, pasos]) => {
      const [titulo, nota] = SECCIONES[sec] || [sec, ""];
      return `<div class="seccion"><h3>${esc(titulo)}</h3>${nota ? `<p class="nota">${esc(nota)}</p>` : ""}${pasos.map((p) =>
        `<div class="paso ${p.ok ? "ok" : "fail"}"><span class="icon" aria-label="${p.ok ? "correcto" : "falló"}">${p.ok ? "✓" : "✗"}</span>` +
        `<span>${esc(p.nombre)}</span><span class="t">${p.t} s</span>` +
        (p.detalle ? `<span class="det">${esc(p.detalle)}</span>` : "") + `</div>`).join("")}</div>`;
    }).join("");

    const reg = (rep.registro || []).slice().sort((a, b) => (a.Padre + a.Nombre).localeCompare(b.Padre + b.Nombre));
    $("#registro").innerHTML = `<thead><tr><th>Servicio</th><th>Operación</th><th>Método</th><th>Destino</th></tr></thead><tbody>` +
      reg.map((s) => `<tr><td>${esc(s.Padre)}</td><td>${esc(s.Nombre)}</td><td>${esc(s.Method)}</td><td>${esc((s.Url || "") + s.Ruta)}</td></tr>`).join("") +
      `</tbody>`;
  })
  .catch((e) => { $("#resumenTexto").textContent = "No se pudo cargar el reporte: " + e.message; });
