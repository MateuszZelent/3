#!/usr/bin/env python3
"""Independent Zarr/HDF5 readers after Flush followed by os.Exit, without Close.

Run from the repository root. Requires Go, Python zarr 2.x, h5py and numpy.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile

import h5py
import numpy as np
import zarr

SOURCE = r'''
package main
import("os";"path/filepath";"github.com/mumax/3/tabular";"github.com/mumax/3/hdf5")
func main(){
 root:=os.Args[1]
 for _,format:=range []string{"zarr","hdf5"}{
  w,err:=tabular.New(filepath.Join(root,format),format,[]string{"t","mx"},[]string{"s",""});if err!=nil{panic(err)}
  if err=w.Append([][]float64{{0,1,2},{0.25,0.5,0.75}});err!=nil{panic(err)}
 }
 mw,err:=hdf5.NewMultiWriter(filepath.Join(root,"fields"));if err!=nil{panic(err)}
 for step:=0;step<3;step++{
  if err=mw.SaveArray("m",step,[][][][]float32{{{{float32(step),2}}}},[3]int{2,1,1},1);err!=nil{panic(err)}
  if err=mw.Flush();err!=nil{panic(err)}
 }
 os.Exit(0)
}
'''


with tempfile.TemporaryDirectory(prefix="mumax-independent-readers-") as temp:
    root = Path(temp)
    source = root / "writer.go"
    source.write_text(SOURCE)
    env = dict(os.environ)
    env.setdefault("GOCACHE", "/tmp/mumax-repair-cache")
    subprocess.run(["go", "run", str(source), temp], env=env, check=True)
    expected = {"t": [0, 1, 2], "mx": [0.25, 0.5, 0.75]}
    for fmt in ("zarr", "hdf5"):
        manifest = json.loads((root / fmt / "manifest.json").read_text())
        assert manifest["committed_rows"] == 3
        assert manifest["units"] == ["s", ""]
        if fmt == "zarr":
            group = zarr.open_group(str(root / fmt), mode="r")
            for name, values in expected.items():
                np.testing.assert_array_equal(group[name][:3], values)
                assert group[name].attrs["unit"] == ("s" if name == "t" else "")
        else:
            with h5py.File(root / fmt / "000000000000.h5", "r") as group:
                for name, values in expected.items():
                    np.testing.assert_array_equal(group[name][:3], values)
                    assert group[name].attrs["unit"][0].decode() == ("s" if name == "t" else "")
    with h5py.File(root / "fields/m/m.h5", "r") as group:
        assert set(group) == {"0", "1", "2"}
        for step in range(3):
            np.testing.assert_array_equal(group[str(step)][:].reshape(-1), [step, 2])
    print(json.dumps({"result": "PASS", "zarr": zarr.__version__, "h5py": h5py.__version__,
                      "formats": ["zarr", "hdf5"], "committed_rows": 3,
                      "field_frames_after_repeated_flush": 3, "writer_close_called": False}))
