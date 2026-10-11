"""End-to-end test against the system running in Kubernetes (kind).

    python3 tests/e2e.py [--esb http://localhost:8085] [--reporte build/reporte.json]

1. Waits for every Deployment and checks the bus learned all 9 routes.
2. Runs one order through the whole flow, via the ESB only.
3. Deletes the ESB pod and measures how long until the new pod knows every route
   again (services re-register on their heartbeat), then runs another order.
4. Deletes the restaurante pod: the system recovers, but orders it kept in
   memory are gone, which the test records as a known limitation.
Writes a JSON report the website renders.
"""

import argparse
import json
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone

NS = "esb"
DEPLOYMENTS = ["esb", "cliente", "restaurante", "repartidor"]
OPERACIONES = 9


def kubectl(*args, check=True):
    r = subprocess.run(["kubectl", "-n", NS, *args], capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeError(f"kubectl {' '.join(args)}: {r.stderr.strip()}")
    return r.stdout


class ESB:
    def __init__(self, url):
        self.url = url

    def llamar(self, metodo, ruta, cuerpo=None, timeout=5):
        """Returns (status, json body, pod that answered); status 0 = no connection."""
        data = json.dumps(cuerpo).encode() if cuerpo is not None else None
        req = urllib.request.Request(self.url + ruta, data=data, method=metodo,
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.status, json.loads(r.read() or b"null"), r.headers.get("X-ESB-Pod")
        except urllib.error.HTTPError as e:
            cuerpo = e.read()
            try:
                cuerpo = json.loads(cuerpo)
            except ValueError:
                cuerpo = cuerpo.decode(errors="replace").strip()
            return e.code, cuerpo, e.headers.get("X-ESB-Pod")
        except OSError:  # refused, reset or timed out while pods are replaced
            return 0, None, None

    def operaciones(self):
        status, cuerpo, pod = self.llamar("GET", "/servicios", timeout=1)
        return (len(cuerpo or []) if status == 200 else None), pod


class Reporte:
    def __init__(self):
        self.pasos = []
        self.metricas = {}
        self.fallos = 0
        self.inicio = time.time()

    def paso(self, seccion, nombre, ok, detalle="", segundos=None):
        self.pasos.append({"seccion": seccion, "nombre": nombre, "ok": bool(ok),
                           "detalle": detalle, "segundos": segundos,
                           "t": round(time.time() - self.inicio, 1)})
        print(f"  {'ok  ' if ok else 'FAIL'} [{seccion}] {nombre}" + (f" — {detalle}" if detalle else ""))
        if not ok:
            self.fallos += 1


def esperar(cond, timeout, intervalo=0.2):
    t0 = time.time()
    while time.time() - t0 < timeout:
        valor = cond()
        if valor:
            return valor, round(time.time() - t0, 1)
        time.sleep(intervalo)
    return None, round(time.time() - t0, 1)


class Sonda:
    """Sends a real request through the ESB every 100 ms and records the results,
    to measure what users would see while a pod is replaced."""

    def __init__(self, esb):
        self.esb, self.resultados, self.latencias, self._parar = esb, [], [], threading.Event()
        self._hilo = threading.Thread(target=self._correr, daemon=True)

    def _correr(self):
        t0 = time.time()
        while not self._parar.is_set():
            inicio = time.time()
            status, cuerpo, _ = self.esb.llamar("POST", "/restaurante_recibir_pedido",
                                                {"idMenu": 1, "idCliente": 1, "IdEstado": 0}, timeout=12)
            self.latencias.append(time.time() - inicio)
            pedido = cuerpo.get("id") if status == 200 and isinstance(cuerpo, dict) else None
            self.resultados.append((round(time.time() - t0, 2), status, time.time(), pedido))
            time.sleep(0.1)

    def __enter__(self):
        self._hilo.start()
        return self

    def __exit__(self, *_):
        self._parar.set()
        self._hilo.join()

    def resumen(self):
        fallos = [t for t, st, *_ in self.resultados if st != 200]
        return {"peticiones": len(self.resultados), "fallidas": len(fallos),
                "ventana_s": round(fallos[-1] - fallos[0], 2) if fallos else 0,
                "codigos": sorted({st for _, st, *_ in self.resultados if st != 200}),
                "latencia_max_ms": round(max(self.latencias, default=0) * 1000),
                "latencia_mediana_ms": round(sorted(self.latencias)[len(self.latencias) // 2] * 1000) if self.latencias else 0}


def pedido_completo(esb, rep, seccion):
    """One order through the whole flow. Returns the order id or None."""
    def paso(nombre, metodo, ruta, cuerpo, espera):
        status, resp, _ = esb.llamar(metodo, ruta, cuerpo)
        mensaje = resp.get("message", "") if isinstance(resp, dict) else str(resp)
        ok = status == 200 and espera in mensaje
        rep.paso(seccion, nombre, ok, f"{metodo} {ruta} → {status} “{mensaje.strip()}”")
        return resp if ok else None

    resp = paso("el cliente pide comida", "POST", "/cliente_solicitar_pedido", {"id": 1}, "pedidio realizado")
    if not resp:
        return None
    n = resp["id"]
    pasos = [
        ("el restaurante lo tiene pendiente", "GET", "/restaurante_estado_restaurante", {"id": n}, "pendiente"),
        ("el restaurante avisa que está listo", "POST", "/restaurante_pedido_listo", {"id": n}, "listo"),
        ("el repartidor va en camino", "GET", "/repartidor_estado", {"id": n}, "en camino"),
        # 2021: the ESB re-encoded this body as {"id":...} and the courier never got idPedido.
        ("el repartidor lo marca entregado", "POST", "/repartidor_marcar_pedido", {"idPedido": n}, f"id pedido: {n} pedido completado"),
        # "ya fue entreagado": the courier's message keeps its 2021 typo.
        ("el cliente ve que fue entregado", "GET", "/cliente_estado_repartidor", {"id": n}, "ya fue"),
    ]
    for p in pasos:
        if not paso(*p):
            return None
    return n


def matar(deployment):
    pod = kubectl("get", "pod", "-l", f"app={deployment}", "-o", "jsonpath={.items[0].metadata.name}")
    kubectl("delete", "pod", pod, "--wait=false")
    return pod


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--esb", default="http://localhost:8085")
    ap.add_argument("--reporte", default="build/reporte.json")
    args = ap.parse_args()
    esb, rep = ESB(args.esb), Reporte()

    print("Arranque")
    for d in DEPLOYMENTS:
        r = subprocess.run(["kubectl", "-n", NS, "rollout", "status", f"deploy/{d}", "--timeout=120s"],
                           capture_output=True, text=True)
        rep.paso("arranque", f"deployment {d} listo", r.returncode == 0)
    n, seg = esperar(lambda: esb.operaciones()[0] == OPERACIONES, 30)
    rep.paso("arranque", f"el ESB conoce las {OPERACIONES} operaciones", n, f"en {seg} s")
    reinicios = sum(int(x) for x in kubectl("get", "pods", "-o",
                    "jsonpath={.items[*].status.containerStatuses[0].restartCount}").split())
    rep.paso("arranque", "ningún pod se reinició al arrancar", reinicios == 0,
             "en 2021 los servicios fallaban si el ESB no estaba listo y dependían de reinicios")

    print("Pedido")
    primero = pedido_completo(esb, rep, "pedido")

    print("Caos: ESB")
    _, viejo = esb.operaciones()
    sonda = Sonda(esb).__enter__()
    time.sleep(1)
    t0 = time.time()
    matado = matar("esb")
    # Recovered = a *different* ESB pod answers and already knows every route again.
    def recuperado():
        ops, pod = esb.operaciones()
        return pod if pod and pod != viejo and ops == OPERACIONES else None
    nuevo, _ = esperar(recuperado, 60)
    segundos = round(time.time() - t0, 1)
    rep.paso("caos-esb", f"se eliminó el pod {matado}", True)
    rep.paso("caos-esb", "un pod nuevo del ESB reaprendió todas las rutas", nuevo,
             f"{nuevo} con {OPERACIONES} operaciones a los {segundos} s, sin reiniciar ningún servicio", segundos)
    rep.metricas["recuperacion_esb_s"] = segundos
    time.sleep(2)
    sonda.__exit__()
    rep.metricas["sonda_esb"] = r = sonda.resumen()
    rep.paso("caos-esb", "peticiones que fallaron durante el reemplazo", True,
             f"{r['fallidas']} de {r['peticiones']}; la más lenta tardó {r['latencia_max_ms']} ms (mediana {r['latencia_mediana_ms']} ms)")
    segundo = pedido_completo(esb, rep, "caos-esb")

    print("Caos: restaurante")
    sonda = Sonda(esb).__enter__()
    time.sleep(1)
    t0 = time.time()
    matado = matar("restaurante")
    rep.paso("caos-restaurante", f"se eliminó el pod {matado}", True)
    # The old pod keeps answering for a moment while it terminates; wait until it is gone.
    _, seg = esperar(lambda: subprocess.run(["kubectl", "-n", NS, "get", "pod", matado],
                                            capture_output=True).returncode != 0, 60, intervalo=0.5)
    rep.paso("caos-restaurante", "el pod viejo terminó", True, f"a los {seg} s")
    def pide():
        status, resp, _ = esb.llamar("POST", "/cliente_solicitar_pedido", {"id": 2}, timeout=2)
        return resp if status == 200 else None
    resp, _ = esperar(pide, 60, intervalo=0.5)
    segundos = round(time.time() - t0, 1)
    rep.paso("caos-restaurante", "el restaurante nuevo acepta pedidos", resp,
             f"pedido {resp and resp.get('id')} a los {segundos} s", segundos)
    rep.metricas["recuperacion_restaurante_s"] = segundos
    time.sleep(2)
    sonda.__exit__()
    rep.metricas["sonda_restaurante"] = r = sonda.resumen()
    rep.paso("caos-restaurante", "peticiones que fallaron durante el reemplazo", True,
             f"{r['fallidas']} de {r['peticiones']}; la más lenta tardó {r['latencia_max_ms']} ms (mediana {r['latencia_mediana_ms']} ms)")
    # Orders live in the restaurante's memory: the new pod starts numbering from 0
    # again and knows nothing about earlier orders. Recorded, not hidden.
    antes = [p for _, st, ts, p in sonda.resultados if p is not None and ts < t0]
    despues = [p for _, st, ts, p in sonda.resultados if p is not None and ts > t0 + segundos]
    if antes and despues:
        rep.paso("caos-restaurante", "los pedidos anteriores se perdieron (estado en memoria)",
                 min(despues) < max(antes),
                 f"antes del reinicio iba en el pedido {max(antes)}; el pod nuevo volvió a empezar en {min(despues)}")

    pods = json.loads(kubectl("get", "pods", "-o", "json"))["items"]
    version = json.loads(subprocess.run(["kubectl", "version", "-o", "json"], capture_output=True, text=True).stdout)
    reporte = {
        "generado": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "kubernetes": version.get("serverVersion", {}).get("gitVersion"),
        "duracion_s": round(time.time() - rep.inicio, 1),
        "metricas": rep.metricas,
        "pasos": rep.pasos,
        "pods": [{"nombre": p["metadata"]["name"], "listo": all(c["ready"] for c in p["status"].get("containerStatuses", [])),
                  "reinicios": sum(c["restartCount"] for c in p["status"].get("containerStatuses", [])),
                  "imagen": p["spec"]["containers"][0]["image"]} for p in pods],
        "registro": esb.llamar("GET", "/servicios")[1],
    }
    with open(args.reporte, "w") as f:
        json.dump(reporte, f, ensure_ascii=False, indent=1)
    print(f"\n{rep.fallos} failure(s); report in {args.reporte}")
    sys.exit(1 if rep.fallos else 0)


if __name__ == "__main__":
    main()
