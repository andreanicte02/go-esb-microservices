"""Assembles the GitHub Pages site: the page plus the e2e report.

    python3 scripts/build_site.py [site] [--reporte build/reporte.json]

In GitHub Actions it also records the link to the run that produced the report.
"""

import argparse
import json
import os
import shutil
from pathlib import Path

RAIZ = Path(__file__).resolve().parent.parent

ap = argparse.ArgumentParser()
ap.add_argument("salida", nargs="?", default="site")
ap.add_argument("--reporte", default="build/reporte.json")
args = ap.parse_args()

out = Path(args.salida)
shutil.rmtree(out, ignore_errors=True)
shutil.copytree(RAIZ / "web", out)

reporte = json.loads(Path(args.reporte).read_text())
if os.environ.get("GITHUB_RUN_ID"):
    reporte["run_url"] = (f"{os.environ['GITHUB_SERVER_URL']}/{os.environ['GITHUB_REPOSITORY']}"
                          f"/actions/runs/{os.environ['GITHUB_RUN_ID']}")
(out / "reporte.json").write_text(json.dumps(reporte, ensure_ascii=False))
print(f"site written to {out}/")
