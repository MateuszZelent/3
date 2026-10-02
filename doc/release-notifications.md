# Startup release notifications

Official versioned builds check GitHub's latest stable release once in a background
Go routine when starting a simulation or interactive session. The checker never
uses the engine injection queue, CUDA context, or numerical stepping loop. It
never downloads or installs an executable automatically.

A newer numeric version (including the date suffix for equal versions) produces
one notice per process in stdout, the simulation's `log.txt`, and UI console
history. The notice gives the installed version, available version, publication
date in UTC, release link, and `mumax3 --update` command. Notices received before
the output log opens are included when its history is flushed. Notifications
arriving after the engine log closes are discarded.

The network request has a 3-second deadline, no retries, no redirects, and a
bounded 1 MiB response. Offline operation, malformed responses, rate limits,
missing binary assets, drafts, and prereleases remain silent and do not change
simulation exit status. Shutdown cancels the request without waiting. A very
short simulation may finish before an uncached request returns.

Metadata is stored under `os.UserCacheDir()/mumax3/release-notice` (on Linux,
`$XDG_CACHE_HOME` or `~/.cache`). A successful check is reused for 6 hours; failed
attempts back off for 1 hour. Atomic replacement protects cache readers. A
nonblocking OS file lock coalesces concurrent processes, including queued jobs;
process termination releases the lock automatically. Processes that cannot obtain
the lock reuse any existing cached release and never wait for the checker.
An inaccessible cache disables network checks to avoid unthrottled batch traffic.
No simulation data is sent; the request fetches public GitHub release metadata.

Use `mumax3 --check-updates=false simulation.mx3` to disable all checking and
notifications. This option is forwarded to queued workers. Version/help, explicit
update commands, vet and internal CUDA tests do not start automatic checking.
Unversioned development builds skip the check because their release ordering
cannot be determined reliably.

Validation: updater tests cover numeric ordering, caching/expiry, failure backoff,
concurrent checks, HTTP cancellation, cached asynchronous notification, and an
inaccessible cache. The engine concurrency test checks history/file logging and
late-notification suppression under the Go race detector. A local executable was
also run on an RTX 4080 SUPER with a controlled newer-release cache: exactly one
notice appeared in stdout and `log.txt`; the disabled run had neither. The cached
newer tag in this test is a fixture, not evidence of a published release.
