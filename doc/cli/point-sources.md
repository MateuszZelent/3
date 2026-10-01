# Surface antenna profiles

The script interpreter supports string equality (`==`) and inequality (`!=`).
A simulation can select an antenna profile with `point_source := "sinc"`,
`"gauss"`, or `"point"` and branch using ordinary `if` statements.

The local YIG example in `local_tests/test.mx3` uses:

- `sinc`: `sinc(kCut*rho)`, optionally a separable XZ sinc product.
- `gauss`: `exp(-rho*rho/(2*gaussSigma*gaussSigma))`; sigma in metres.
- `point`: uniform disk with `rho <= pointDiameter/2`; diameter in metres.

All profiles occupy only `jAntenna := Ny-1`. Keep the vector mask at the full
`Nx, Ny, Nz` size: `B_ext.Add` resamples differently sized masks, so a one-row
mask would spread across Y. Spatial values are sampled at cell centres; a disk
smaller than the nearest cell-centre distance may excite no cells.

The time envelope remains sinc for all spatial profiles. In preparation mode,
`previewAntennaPeak := true` leaves static RF at its peak for inspection without
advancing time. Dynamics uses the time-dependent envelope instead.

Inspect the **Z component** of `B_ext`, in a single-layer section. On XZ select
Y=Ny-1; on YZ select X through the antenna centre. The uniform Y bias is 126 mT,
whereas peak RF is 0.1 mT. `print(B_ext)` reports a volume average, not a local
field. The local example is ignored and is not a bundled release asset.

Verification: all three profiles were evaluated on GPU at 500x4x500 cells
(5 nm cells, full XZ antenna resolution). Saved XZ fields match the analytical
profiles; adjacent Y layers are exactly zero. A separate 32-cube dynamics smoke
check verified the sinc pulse maximum and removal of RF after the pulse.
These checks do not validate propagation in the full 500-cube YIG model.
