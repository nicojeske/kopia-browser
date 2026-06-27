# Changelog

## [1.1.0](https://github.com/nicojeske/kopia-browser/compare/v1.0.0...v1.1.0) (2026-06-27)


### Features

* add volume navigation layer (namespace to volume to snapshots) ([7e343da](https://github.com/nicojeske/kopia-browser/commit/7e343da95e06dd48d46d07b097b8b5b16fc37d2a))
* file-type icons with category colours in browse view ([b960c35](https://github.com/nicojeske/kopia-browser/commit/b960c357637544375351209fe05bff4eb06e43d4))
* M0 scaffold — config, HTTP server, embedded UI ([9662a93](https://github.com/nicojeske/kopia-browser/commit/9662a93cab070e3caf76118c27a5e2ca77a99bdd))
* M1 — list namespaces + snapshots ([f8ec882](https://github.com/nicojeske/kopia-browser/commit/f8ec882d07cef2700f0b39485bb25ea3d83e5fa3))
* M2 — browse dir tree with htmx SPA navigation ([8fd7b9d](https://github.com/nicojeske/kopia-browser/commit/8fd7b9d04a3c3e4871d60c4c24a14499a2a3bc45))
* M3 — download single file from snapshot ([99f6cb2](https://github.com/nicojeske/kopia-browser/commit/99f6cb251e09d66e192bd6c82e93e828ba581b35))
* M4 — download folder as plain tar ([b54a2ee](https://github.com/nicojeske/kopia-browser/commit/b54a2ee28c3b408b5058f33cfa8e29cb0a3cc94e))
* M5 — new dark-theme UI (redesigned templates, self-hosted fonts) ([31213ff](https://github.com/nicojeske/kopia-browser/commit/31213ffadf433cd2170e55f73d7601c2f59d380d))
* M5 — styled error page and persistent sidebar nav ([a34a62f](https://github.com/nicojeske/kopia-browser/commit/a34a62fa34db1fb96cb5af5ddb4770c774ebdde2))
* M6 — multi-stage Dockerfile for distroless container image ([bcc8193](https://github.com/nicojeske/kopia-browser/commit/bcc819350fe496d9b5820cb5ccd9b919dd300c73))
* M7 — dashboard stats, enriched sidebar, background stats cache ([5c2c47e](https://github.com/nicojeske/kopia-browser/commit/5c2c47e43056dd4b89f7af8a0001e08a8692420a))
* M8 — GitHub Actions CI/CD (build, test, Docker publish) ([66114d7](https://github.com/nicojeske/kopia-browser/commit/66114d713f26055575d84b2d1f4c8d8ac68a1be3))
* meaningful snapshot rows with retention badges and copy ID ([a944470](https://github.com/nicojeske/kopia-browser/commit/a944470da19aa002c30c43c93d1823ab23d71839))
* migrate to log/slog with levels + cache progress logging ([eab4137](https://github.com/nicojeske/kopia-browser/commit/eab4137ff4abc44c8ef097a3626698516ca6686c))
* persist stats cache to disk across restarts ([3fd63ea](https://github.com/nicojeske/kopia-browser/commit/3fd63ea6c6057fae0f4aba0cb921ea6d193a0455))
* retention badges, duration column, and snapshot label improvements ([3d966c4](https://github.com/nicojeske/kopia-browser/commit/3d966c435eabdaff9cac0bfe777ea1a58314b9a6))
* show folder sizes in snapshot browse view ([0139f7f](https://github.com/nicojeske/kopia-browser/commit/0139f7fd01831de6a807073b6c681c1c743707f7))
* sortable columns + alignment fix in snapshot browse view ([b8212cb](https://github.com/nicojeske/kopia-browser/commit/b8212cb003565f8af59c5fd21acd43ec7dc799a9))
* update M6 milestone for Docker implementation ([ade7737](https://github.com/nicojeske/kopia-browser/commit/ade773754d06c1498c4dec9ea8aa8b187c7e3641))
* update stats refresh interval to 60 minutes ([2211a7f](https://github.com/nicojeske/kopia-browser/commit/2211a7f34bec7e1ba5df68d0df5f56b28328e049))


### Bug Fixes

* enable kopia content disk cache (was silently disabled) ([2ea5878](https://github.com/nicojeske/kopia-browser/commit/2ea587881b8e60faef7b1b0698d941bcb78d7550))
* extract volume from source.path for data-mover snapshots ([38f6683](https://github.com/nicojeske/kopia-browser/commit/38f6683533612d1bb2be7f38decdb0f813dcac0a))
* left-align volumes table headers and data cells ([d158df3](https://github.com/nicojeske/kopia-browser/commit/d158df3ae47a1867c96b10a6c53ea161d8552f90))
* M5 — update E2E selectors for redesigned CSS classes ([6f50aca](https://github.com/nicojeske/kopia-browser/commit/6f50aca0b2feb26164a8aecc1847844bab51a1b6))
* right-align table column headers when col-r is applied ([1f37c98](https://github.com/nicojeske/kopia-browser/commit/1f37c98e067f8114b2192b13869344d45b4ec461))
* skip initial stats refresh when cache is still fresh ([597b4fc](https://github.com/nicojeske/kopia-browser/commit/597b4fc14299cbd590c4b987b9ece9389c75b065))
* snapshot ordering, Latest stat, and file count in volume view ([68c3dbb](https://github.com/nicojeske/kopia-browser/commit/68c3dbb739213b81c0d166386b873880cf5a3150))
* update volumes screenshot for improved clarity ([d165c69](https://github.com/nicojeske/kopia-browser/commit/d165c698f89a68d79b6bc03253092cc3da638ad5))


### Performance Improvements

* buffer tar writes and use 1MB copy buffer for faster downloads ([a680cd7](https://github.com/nicojeske/kopia-browser/commit/a680cd7b10c35b0020ef9926dd46515ea55a59e0))
