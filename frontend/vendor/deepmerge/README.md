# Hardened deepmerge build dependency

This private package preserves the deepmerge 4.3.1 API used by the Svelte Vite plugin. It is pinned as a local dependency because upstream has no published fix for [GHSA-gcqx-w48m-w2h4](https://github.com/advisories/GHSA-gcqx-w48m-w2h4).

The upstream report is disputed (see the linked issue). This is audit-driven local hardening of a reproduced prototype-poisoning behavior while the GHSA remains in npm audit data; it does not rely on the disputed CVE classification.

Provenance: `index.cjs` comes from `package/dist/cjs.js` in the official [deepmerge 4.3.1 npm archive](https://registry.npmjs.org/deepmerge/-/deepmerge-4.3.1.tgz). Its SHA-512 integrity is `sha512-3sUqbMEc77XqpdNO7FRyRog+eW3ph+GYCbj+rK+uYyRMuwsVy0rMiVtPn+QJlKFvWP/1PYpapqYn0Me2knFn+A==`. The upstream type declarations and MIT license are retained.

The local patch rejects `__proto__`, `constructor`, and `prototype` in both input-copy paths, defines result properties without invoking inherited setters, and rejects source properties when target membership checks throw. It follows the remediation described in the [upstream security report](https://github.com/TehShrike/deepmerge/issues/273). Normal object prototypes and existing array/custom-merge behavior are preserved.

`npm run test:unit` runs the installed dependency's prototype-poisoning regressions and configuration-merge compatibility checks before the frontend unit suite. No install or build script is required for this package. Remove the local dependency and this directory after a fixed upstream release passes these tests and `npm audit --audit-level=low`.
