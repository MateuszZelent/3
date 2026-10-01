# Release picker validation — 2026-10-01

`--update` / `-u` now lists published, installable GitHub releases with their tags
and actual publication dates in UTC, newest publication first. The latest,
installed and prerelease statuses are marked. A number or exact tag selects a
release; Enter selects GitHub latest, q cancels. `--update-list` performs no
installation. `--update-version TAG|latest` is the unattended path.

## Verified locally

- `go test -vet=off ./updater ./cmd/mumax3` passes in the supported CUDA container.
- Tests cover pagination, publication ordering, duplicate tags, draft/missing
  binary filtering, prerelease marking, installed/latest labels, exact tag
  resolution, API errors, cancellation, invalid input, EOF and existing atomic
  replacement/permission preservation tests.
- The built CLI read the real GitHub catalog: 18 installable releases; latest
  v3.12.12-20261001 was shown with publication 2026-10-01 10:40 UTC.
- Selecting row 2 through stdin atomically installed v3.12.11-20261001 over a
  temporary copy. Running that copy with `--version` reported v3.12.11-20261001,
  commit 41df1a8f. SHA256:
  de449438289aa9bbd9d24ec53fe56b100964ea3fb81029d83dd0e2446652c093.
- `--update-version v3.12.12-20261001` installed exactly that release over another
  temporary copy; `--version` reported v3.12.12-20261001, commit 9e86d79c. SHA256:
  1ec99c06e1cfb8449aaf520de1deab67fbc4dc9e58a6112de0e326b32f0f3d3c.
- The final build accepted q and exited successfully when mounted read-only,
  confirming cancellation does not try to replace the executable.

The real installation tests used only disposable copies under
`/tmp/3-update-picker-check`, not the user's HPC installation. CLI updates do not
initialize or run a simulation. The local executable is
`build/mumax3-update-picker`; v3.12.12 still contains the previous updater until
a new release includes these source changes.
