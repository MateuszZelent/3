# GitHub security and pull-request cleanup

Scope: MateuszZelent/3. Initial snapshot: 22 open Dependabot alerts and eight
open pull requests; no open non-PR issues. Code scanning reported no analysis;
secret scanning reported disabled. Those services did not provide security
coverage and are not represented as clean scans.

All npm alerts are addressed by the production lockfile and corrected minimum
versions/overrides: SvelteKit 2.70.3, Vitest/mocker 4.1.11, devalue 5.9.4,
brace-expansion 1.1.21 and 5.0.12, js-yaml 4.3.2, nanoid 3.3.18. Existing overrides
for older vulnerable versions were updated, rather than performing the unrelated
ESLint 10 migration. npm audit includes development dependencies and reports zero
vulnerabilities after installation.

Echo is upgraded to 4.15.3. The source scan additionally found two reachable SSH
vulnerabilities, GO-2026-6354 and GO-2026-6355, fixed by x/crypto 0.56.0. Its
compatible x/* dependencies were updated. Both go.mod and the CUDA build image
now require Go 1.26.8, a supported toolchain with standard-library security fixes
(https://go.dev/doc/devel/release). The source and executable scans report zero
vulnerabilities in reachable code or imported packages. They additionally identify
the unmaintained x/crypto/openpgp package at module level; this application does
not import or include it. The SSH dependency cannot be removed to suppress that
unrelated package's advisory.

The numerical solver code and CUDA version remain unchanged. Existing compiled
releases retain their original dependencies; the security fixes are shipped in
v3.12.19-20261002. Production frontend assets are rebuilt with the corrected lock.

| PR | Decision |
| --- | --- |
| #21 | Superseded by devalue 5.9.4 in the consolidated security fix. |
| #20 | Superseded by brace-expansion 5.0.12; the 1.x copy is also fixed. |
| #17 | Superseded by js-yaml 4.3.2 and correction of the old override. |
| #16 | Superseded by Vitest and mocker 4.1.11. |
| #15 | Superseded by Echo 4.15.3. |
| #14 | Accepted: applied its actual-port validation to the ready-event test. |
| #9 | Superseded by the newer compatible SvelteKit 2.70.3. |
| #7 | Declined as proposed: its security fix is included; the unrelated major ESLint/plugin migration is unnecessary for resolving these alerts. |

CI and release builds audit all npm severities and scan the built solver using
pinned govulncheck 1.8.0. Dependabot groups compatible and security updates to
reduce duplicate PR traffic; security alerts remain enabled.

Validation: 44 frontend tests; Svelte check zero errors/warnings; production
frontend build; updater/background engine logging under the race detector;
CLI, WebUI, mag and util tests in the supported CUDA container on Go 1.26.8;
source and actual-binary vulnerability scans. Initial snapshots and scan evidence
are retained alongside this report. Resolved PR records remain in GitHub history;
their obsolete source branches are deleted after the fix reaches master.
