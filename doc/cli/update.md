# Updating mumax3

`mumax3 --update` (or `mumax3 -u`) lists published GitHub releases containing a
mumax3 binary, with their release tag and publication date in UTC. The table marks
the installed version, GitHub's latest release and prereleases. Releases are
ordered by publication date; prereleases are explicitly marked.

Enter a row number or exact tag to install that release. Press Enter to install
the release marked `latest`, or enter `q` to cancel. Choosing an older release
uses that exact release's binary. Invalid input prompts again and does not modify
the executable. EOF without a selection fails without installing anything.

```sh
mumax3 --update                         # interactive release picker
mumax3 --update-list                    # list versions/dates; do not install
mumax3 --update-version latest          # unattended update to GitHub latest
mumax3 --update-version v3.12.12-20261001 # install this exact published version
mumax3 --version                        # verify the installed build afterward
```

The update resolves symlinks and atomically replaces the executable while keeping
its permissions. A failed download leaves the existing binary intact. Only the
binary is updated; existing CUDA runtime libraries are not replaced. The new
version is reported on successful installation. Scripts that previously used
`--update` without terminal input should use `--update-version latest` instead.
