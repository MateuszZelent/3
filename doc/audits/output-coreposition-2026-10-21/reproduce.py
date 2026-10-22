#!/usr/bin/env python3
"""Historical audit probes pinned to the pre-fix commit; no CUDA or solver build.

Run from any directory with Python 3 and a local Go toolchain. Generated files
and probe output go to a temporary directory. This is not an engine/GPU test.
"""

import os
from pathlib import Path
import signal
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[3]
BASELINE = "0efd540073bbad7ec2f1ef3ed86d645441b61b6a"


def baseline(path):
    return subprocess.check_output(["git", "show", f"{BASELINE}:{path}"], cwd=ROOT, text=True)



def function(source, signature):
    start = source.index(signature)
    brace = source.index("{", start)
    depth = 1
    end = brace + 1
    while depth:
        depth += (source[end] == "{") - (source[end] == "}")
        end += 1
    return source[start:end]


MAIN = r'''
package main

import ("fmt"; "math"; "os"; "time")

const (X = 0; Y = 1; Z = 2; FilePerm = 0666)
type field struct { values [][][]float32; size [3]int }
func (f *field) Buffer() *field { return f }
func (f *field) Comp(int) *field { return f }
func (f *field) HostCopy() *field { return f }
func (f *field) Scalars() [][][]float32 { return f.values }
func (f *field) Size() [3]int { return f.size }
type mesh struct{}
func Mesh() mesh { return mesh{} }
func (mesh) CellSize() [3]float64 { return [3]float64{1e-9, 1e-9, 1e-9} }
var M field
func GetShiftPos() float64 { return 0 }
func Remove(p string) error { return os.RemoveAll(p) }
func Read(p string) ([]byte,error) { return os.ReadFile(p) }
func Touch(p string) error { return localTouch(p) }
func AppendSize(p string, b []byte, n int64) error { return localAppend(p,b,n) }
type counter int
func (c *counter) Load() int { return int(*c) }
func (c *counter) Add(n int) { *c += counter(n) }
var queLen counter
var saveQue chan func()

func makeField(nx, ny int, value func(float64,float64) float32) {
    M = field{size:[3]int{nx,ny,1}, values:make([][][]float32,1)}
    M.values[0] = make([][]float32,ny)
    for y:=0;y<ny;y++ {
        M.values[0][y] = make([]float32,nx)
        for x:=0;x<nx;x++ {
            M.values[0][y][x] = value(float64(x)-float64(nx)/2+0.5,float64(y)-float64(ny)/2+0.5)
        }
    }
}
func report(label string) {
    defer func(){ if p:=recover();p!=nil { fmt.Printf("%s panic=%v\n",label,p) } }()
    p:=corePos()
    fmt.Printf("%s x_nm=%g y_nm=%g polarization=%g finite=%v\n",label,p[0]*1e9,p[1]*1e9,p[2], !math.IsNaN(p[0])&&!math.IsNaN(p[1])&&!math.IsInf(p[0],0)&&!math.IsInf(p[1],0))
}
func cores() {
    const cx,cy=11.25,-7.75
    fmt.Println("grid=128x96x1 cell=1nm expected_center_nm=(11.25,-7.75)")
    for _,w:=range []float64{2,4,8,16,24,32} {
        for _,pol:=range []float64{-1,1} {
            // Normalized mz from the NeelSkyrmion/BlochSkyrmion profile.
            // float32 CPU normalization models stored input, not CUDA rounding.
            makeField(128,96,func(x,y float64) float32 {
                x-=cx; y-=cy
                r:=math.Hypot(x,y)
                mz:=2*pol*(math.Exp(-r*r/(w*w))-0.5)
                mx,my:=float32(x/r*(1-math.Abs(mz))),float32(y/r*(1-math.Abs(mz)))
                z:=float32(mz)
                return z/float32(math.Sqrt(float64(mx*mx+my*my+z*z)))
            })
            report(fmt.Sprintf("skyrmion w_nm=%g diameter_mz0_nm=%.6f pol=%g",w,2*w*math.Sqrt(math.Log(2)),pol))
        }
    }
    makeField(128,96,func(x,y float64) float32 { return float32(math.Exp(-((x-cx)*(x-cx)+(y-cy)*(y-cy))/16)) })
    report("localized_vortex_like_peak")
    makeField(128,96,func(x,y float64) float32 { if math.Hypot(x-cx,y-cy)<12 {return -1};return 1 })
    report("flat_core_and_background")
    makeField(128,96,func(x,y float64) float32 { return 1 })
    report("uniform_background")
    makeField(2,2,func(x,y float64) float32 { return 1 })
    report("grid_2x2")
}
func writer(mode,dir string) {
    log,e:=Create(dir+"/log.txt");if e!=nil {panic(e)}
    table,e:=Create(dir+"/table.txt");if e!=nil {panic(e)}
    cleanup:=func(){if e:=table.Flush();e!=nil {panic(e)};if e:=log.Close();e!=nil {panic(e)}}
    defer cleanup()
    fmt.Fprintln(log,"// buffered audit log")
    fmt.Fprintln(table,"# t (s)\tmx ()")
    if e:=table.Flush();e!=nil {panic(e)}
    fmt.Fprintln(table,"0\t1")
    if mode=="flush" { drainOutput() }
    fmt.Println("ready")
    if mode=="clean" {return}
    if mode=="fatal" {os.Exit(1)}
    for {time.Sleep(time.Second)}
}
func main(){if os.Args[1]=="core" {cores()} else {writer(os.Args[1],os.Args[2])}}
'''


def main():
    print(f"Historical baseline: {BASELINE}", flush=True)
    core = baseline("engine/ext_corepos.go")
    local = baseline("httpfs/httpfs.go")
    reader = baseline("httpfs/reader.go")
    asyncio = baseline("engine/asyncio.go")
    body = MAIN + "\n" + core[core.index("func corePos()") :]
    body += "\n" + function(local, "func localTouch(")
    body += "\n" + function(local, "func localAppend(")
    body += "\n" + function(asyncio, "func drainOutput(")
    with tempfile.TemporaryDirectory(prefix="mumax-output-core-audit-") as tmp:
        work = Path(tmp)
        (work / "main.go").write_text(body)
        (work / "reader.go").write_text(reader.replace("package httpfs", "package main", 1))
        env = dict(os.environ, GOTOOLCHAIN="local", GO111MODULE="off", GOCACHE="/tmp/mumax-output-core-audit-gocache")
        subprocess.run(["go", "build", "-o", str(work / "probe"), "main.go", "reader.go"], cwd=work, env=env, check=True)
        print("Extracted corePos/interpolate_maxpos/abs; CPU field stubs; no GPU", flush=True)
        subprocess.run([str(work / "probe"), "core"], check=True)
        print("\nExtracted httpfs writer and local filesystem functions; simulated log/table writes", flush=True)
        for mode, sig in [("clean", None), ("fatal", None), ("live", signal.SIGINT), ("live", signal.SIGTERM), ("flush", signal.SIGTERM)]:
            directory = work / f"{mode}-{sig}"
            directory.mkdir()
            child = subprocess.Popen([str(work / "probe"), mode, str(directory)], stdout=subprocess.PIPE, text=True)
            try:
                if child.stdout.readline().strip() != "ready":
                    raise RuntimeError("probe did not become ready")
                if sig is not None:
                    child.send_signal(sig)
                child.wait(timeout=10)
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait()
                child.stdout.close()
            log = (directory / "log.txt").read_bytes()
            table = (directory / "table.txt").read_bytes()
            print(f"mode={mode} signal={sig.name if sig else 'none'} exit={child.returncode} log_bytes={len(log)} table={table!r}", flush=True)


if __name__ == "__main__":
    main()
