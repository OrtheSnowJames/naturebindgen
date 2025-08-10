import platform
import subprocess
import urllib.request
import tarfile
import zipfile
from pathlib import Path

SRC_FILE = "constants.cpp"
OUT_FILE = {
    "Linux": "libinfer_structs.so",
    "Darwin": "libinfer_structs.dylib",
    "Windows": "infer_structs.dll"
}[platform.system()]

def run(cmd):
    print(">>", " ".join(cmd))
    subprocess.check_call(cmd)

def llvm_config_flags():
    try:
        out = subprocess.check_output(["llvm-config", "--includedir", "--libdir", "--libs", "all", "--system-libs"])
        parts = out.decode().split()
        includes = [f"-I{parts[0]}"]
        libdir = f"-L{parts[1]}"
        libs = [p for p in parts[2:] if p.startswith("-l") or p.startswith("-L")]
        return includes + [libdir] + libs + ["-lclang-cpp"] # TODO: fix constants.py supplying none to the shared object
    except (OSError, subprocess.CalledProcessError):
        return None

def detect_platform_flags():
    sysname = platform.system()
    if sysname == "Darwin":  # macOS
        return [
            "-I/opt/homebrew/opt/llvm/include",
            "-L/opt/homebrew/opt/llvm/lib",
            "-lclang",
        ]
    elif sysname == "Linux":
        llvm_path = Path("/usr/lib/llvm-17")  # adjust version if needed
        if llvm_path.exists():
            return [
                f"-I{llvm_path}/include",
                f"-L{llvm_path}/lib",
                "-lclang-cpp",
                "-lclang", "-lclangTooling", "-lclangFrontend", "-lclangSerialization",
                "-lclangDriver", "-lclangParse", "-lclangSema", "-lclangAnalysis",
                "-lclangEdit", "-lclangAST", "-lclangLex", "-lclangBasic",
                "-lLLVM"
            ]
    elif sysname == "Windows":
        msys_llvm = Path("C:/msys64/mingw64")
        if msys_llvm.exists():
            return [
                f"-I{msys_llvm}/include",
                f"-L{msys_llvm}/lib",
                "-lclang",
            ]
    return None

def download_llvm():
    sysname = platform.system()
    version = "17.0.6"
    if sysname == "Linux":
        url = f"https://github.com/llvm/llvm-project/releases/download/llvmorg-{version}/clang+llvm-{version}-x86_64-linux-gnu-ubuntu-20.04.tar.xz"
        archive = "llvm.tar.xz"
    elif sysname == "Darwin":
        url = f"https://github.com/llvm/llvm-project/releases/download/llvmorg-{version}/clang+llvm-{version}-arm64-apple-darwin21.0.tar.xz"
        archive = "llvm.tar.xz"
    elif sysname == "Windows":
        url = f"https://github.com/llvm/llvm-project/releases/download/llvmorg-{version}/LLVM-{version}-win64.exe"
        archive = "llvm.zip"
    else:
        raise RuntimeError("Unsupported platform for auto-download")

    print(f"Downloading LLVM from {url} ...")
    urllib.request.urlretrieve(url, archive)

    if archive.endswith(".tar.xz"):
        with tarfile.open(archive, "r:xz") as tar:
            tar.extractall("llvm")
    elif archive.endswith(".zip") or archive.endswith(".exe"):
        with zipfile.ZipFile(archive, "r") as z:
            z.extractall("llvm")

    # crude guess: first dir in llvm/ is root
    llvm_root = next(Path("llvm").iterdir())
    return [
        f"-I{llvm_root}/include",
        f"-L{llvm_root}/lib",
        "-lclang",
    ]

def main():
    flags = llvm_config_flags() or detect_platform_flags()
    if not flags:
        print("No LLVM found locally — downloading...")
        flags = download_llvm()

    sysname = platform.system()
    compiler = "clang++" if sysname != "Windows" else "g++"
    shared_flag = "-shared" if sysname != "Darwin" else "-dynamiclib"
    std_flag = "-std=c++17"

    run([compiler, std_flag, SRC_FILE, shared_flag, "-fPIC", "-o", OUT_FILE] + flags)
    print(f"✅ Built {OUT_FILE}")

if __name__ == "__main__":
    main()
