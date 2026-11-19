"""Build standalone desktop and CLI archives. Requires Go and Python 3."""
import hashlib
import os
import pathlib
import plistlib
import subprocess
import shutil
import tarfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent
DIST = ROOT / "dist"
TARGETS = [(system, arch) for system in ("windows", "linux", "darwin")
           for arch in ("amd64", "arm64")]


def build():
    DIST.mkdir(exist_ok=True)
    for system, arch in TARGETS:
        folder = DIST / "build" / f"{system}-{arch}"
        resolved = folder.resolve()
        if resolved.parent != (DIST / "build").resolve() or folder.is_symlink():
            raise RuntimeError("Build directory is outside the release workspace")
        if folder.exists():
            shutil.rmtree(resolved)
        folder.mkdir(parents=True, exist_ok=True)
        env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0")
        cli = folder / ("addonbridge-cli.exe" if system == "windows" else "addonbridge")
        subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(cli), "."],
                       cwd=ROOT, env=env, check=True)
        if system == "windows":
            subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w -H windowsgui",
                            "-o", str(folder / "AddonBridge.exe"), "."], cwd=ROOT, env=env, check=True)
        elif system == "darwin":
            contents = folder / "AddonBridge.app" / "Contents"
            (contents / "MacOS").mkdir(parents=True, exist_ok=True)
            (contents / "MacOS" / "AddonBridge").write_bytes(cli.read_bytes())
            (contents / "Info.plist").write_bytes(plistlib.dumps({
                "CFBundleExecutable": "AddonBridge", "CFBundleIdentifier": "local.addonbridge.desktop",
                "CFBundleName": "AddonBridge", "CFBundleVersion": "0.4.0",
                "CFBundlePackageType": "APPL", "LSUIElement": True,
            }))
        for name in ("README.md", "README.zh-CN.md", "validation.json"):
            (folder / name).write_bytes((ROOT / name).read_bytes())
        files = sorted(p for p in folder.rglob("*") if p.is_file())
        if system == "windows":
            with zipfile.ZipFile(DIST / f"addonbridge-{system}-{arch}.zip", "w", zipfile.ZIP_DEFLATED) as archive:
                for file in files:
                    archive.write(file, file.relative_to(folder))
        else:
            with tarfile.open(DIST / f"addonbridge-{system}-{arch}.tar.gz", "w:gz") as archive:
                for file in files:
                    info = archive.gettarinfo(str(file), str(file.relative_to(folder)))
                    info.mode = 0o755 if file.name in ("addonbridge", "AddonBridge") else 0o644
                    with file.open("rb") as stream:
                        archive.addfile(info, stream)
        print(f"Built {system}-{arch}", flush=True)
    with zipfile.ZipFile(DIST / "addonbridge-source.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        for file in list(ROOT.iterdir()) + list((ROOT / "ui").iterdir()):
            if file.is_file() and (file.parent.name == "ui" or file.suffix in (".go", ".mod", ".sum", ".md", ".py") or file.name in ("validation.json", ".gitignore")):
                archive.write(file, "addon-bridge/" + str(file.relative_to(ROOT)))
    packages = sorted(list(DIST.glob("addonbridge-*.zip")) + list(DIST.glob("addonbridge-*.tar.gz")))
    (DIST / "SHA256SUMS.txt").write_text("".join(
        hashlib.sha256(file.read_bytes()).hexdigest() + "  " + file.name + "\n" for file in packages))


if __name__ == "__main__":
    build()
