# The same work run on the host and inside a sandbox, so the difference is the
# nested VM: arithmetic, starting processes, small files, and one large write.
import json, os, subprocess, time

def timed(fn):
    start = time.perf_counter()
    fn()
    return round(time.perf_counter() - start, 3)

def cpu():
    total = 0
    for i in range(20_000_000):
        total += i

def spawn():
    for _ in range(300):
        subprocess.run(["true"], check=True)

def small_files():
    os.makedirs("/tmp/bench-files", exist_ok=True)
    for i in range(2000):
        path = f"/tmp/bench-files/{i}"
        with open(path, "w") as f:
            f.write("x" * 512)
        with open(path) as f:
            f.read()
    os.sync()

def large_write():
    with open("/tmp/bench-large", "wb") as f:
        block = b"\0" * (1 << 20)
        for _ in range(128):
            f.write(block)
        f.flush()
        os.fsync(f.fileno())
    os.remove("/tmp/bench-large")

print(json.dumps({name: timed(fn) for name, fn in
                  [("cpu", cpu), ("spawn", spawn), ("small_files", small_files), ("large_write", large_write)]}))
