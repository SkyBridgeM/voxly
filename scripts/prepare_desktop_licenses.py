#!/usr/bin/env python3
"""Collect pinned native licenses plus locally available locked Rust/npm licenses."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import tomllib
import urllib.request

repo = Path(__file__).resolve().parent.parent
out = repo / "desktop/src-tauri/resources/licenses"
out.mkdir(parents=True, exist_ok=True)
shutil.copyfile(repo / "LICENSE", out / "VOXLY-LICENSE.txt")
for item in json.loads((repo / "native/third-party-licenses.json").read_text()):
    target = out / item["file"]
    data = target.read_bytes() if target.exists() else urllib.request.urlopen(item["url"], timeout=40).read()
    if hashlib.sha256(data).hexdigest() != item["sha256"]:
        raise SystemExit(f"License checksum mismatch: {item['file']}")
    target.write_bytes(data)

registry = Path(os.environ.get("CARGO_HOME", Path.home() / ".cargo")) / "registry/src"
lock = tomllib.loads((repo / "desktop/src-tauri/Cargo.lock").read_text())
index = []
for package in lock["package"]:
    if not package.get("source", "").startswith("registry+"):
        continue
    name = f"{package['name']}-{package['version']}"
    sources = list(registry.glob(f"*/{name}"))
    if not sources:  # Other-platform crates need not be downloaded for this build.
        continue
    source = sources[0]
    manifest = tomllib.loads((source / "Cargo.toml").read_text()).get("package", {})
    files = [p for p in source.iterdir() if p.is_file() and p.name.lower().startswith(("license", "licence", "copying", "notice", "copyright"))]
    if manifest.get("license-file"):
        files.append(source / manifest["license-file"])
    for file in set(files):
        if file.is_file():
            destination = out / "rust" / name / file.name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(file, destination)
    index.append({"package": name, "license": manifest.get("license", "see included license"), "license_files": sorted({p.name for p in files if p.is_file()})})
(out / "rust-packages.json").write_text(json.dumps(index, indent=2) + "\n")
# Include notices for installed npm packages; these also cover build-time tools.
for manifest in (repo / "desktop/node_modules").glob("**/package.json"):
    source = manifest.parent
    for file in source.iterdir():
        if file.is_file() and file.name.lower().startswith(("license", "licence", "copying", "notice")):
            destination = out / "npm" / source.relative_to(repo / "desktop/node_modules") / file.name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(file, destination)
shutil.copyfile(repo / "native/THIRD-PARTY-NOTICES.md", out / "THIRD-PARTY-NOTICES.md")
print(f"Prepared native licenses and notices for {len(index)} installed locked Rust packages.")
