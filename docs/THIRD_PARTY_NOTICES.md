# Third-party notices

This inventory is checked against `build/dependencies.allow`.

## github.com/go-sql-driver/mysql v1.9.3

The upstream connector uses this maintained `database/sql` driver without modifying its source. It is licensed under Mozilla Public License 2.0. Changes to MPL-covered driver files would require publishing those changed files; this project keeps the module unchanged. Preserve its license notice in releases.

Source: `https://github.com/go-sql-driver/mysql`

## filippo.io/edwards25519 v1.1.0

This transitive driver dependency implements Ed25519 group operations. It uses a BSD 3-Clause license. Preserve its copyright and license notice in distributions.

Source: `https://filippo.io/edwards25519`

## gopkg.in/yaml.v3 v3.0.1

The project is dual-covered by MIT and Apache License 2.0 terms. Copyright holders include Kirill Simonov and Canonical Ltd. The full license text ships in the module source and must be included in release notices when this dependency is distributed.

Source: `https://gopkg.in/yaml.v3`

## gopkg.in/check.v1

This transitive test dependency uses a BSD-style license. Copyright (c) 2010–2013 Gustavo Niemeyer. Its notice must be preserved when applicable to distributed source or binary materials.

Source: `https://gopkg.in/check.v1`

## Audit rule

Every new module must be added to the allowlist only after its exact version, purpose, license, source, maintenance risk, and release-notice requirement are reviewed.
