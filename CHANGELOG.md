# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

## [1.4.0](https://github.com/dsswift/release-damnit/compare/v1.3.0...v1.4.0) (2026-10-20)

### Features

* honor per-package tag-separator from release-please config ([0ecd3bb](https://github.com/dsswift/release-damnit/commit/0ecd3bbb2586e2398ccb0bc0e50ceb371356024e))
* add root package, extra-files and rule keys ([e9e80c5](https://github.com/dsswift/release-damnit/commit/e9e80c5cb6ed203512dc18937d2e52d3414f83e8))
* add check-commit and check-commits ([7cd194e](https://github.com/dsswift/release-damnit/commit/7cd194eb81ca0e85eccbda3d1064db8d02e39b43))
* add land command ([16adcf2](https://github.com/dsswift/release-damnit/commit/16adcf2897fcd7b90172c6596fae2d63a02719eb))
* add hooks command ([54373d4](https://github.com/dsswift/release-damnit/commit/54373d404b62183dd333643f1184ed7b2c1cb255))
* add publish command and action inputs ([241cb5e](https://github.com/dsswift/release-damnit/commit/241cb5ecb5c3c8bce61821ca700b73bfc07f2106))
* add init command for github repos ([6cc1042](https://github.com/dsswift/release-damnit/commit/6cc1042f34f047032e8762933164030652962e44))
* add commit-scope for packages ([b323d8d](https://github.com/dsswift/release-damnit/commit/b323d8da9b25ffc4bbc9c19b7ec4f61248546716))
* analyze each package since its last release ([0132d64](https://github.com/dsswift/release-damnit/commit/0132d64f0eb48638861be2027489b6cd0c42aa6e))

### Bug Fixes

* stop cutting releases for housekeeping commit types ([d3ec868](https://github.com/dsswift/release-damnit/commit/d3ec868c5127673dfe06b44416d25b5172f207e8))
* honor tag-separator in gitlab release tags ([c63fb2b](https://github.com/dsswift/release-damnit/commit/c63fb2b1571c153f97fab1ff1d379a537870df2b))
* skip a package's own VERSION in extra-files ([8bbee71](https://github.com/dsswift/release-damnit/commit/8bbee71f777b9c966f1969bd60f8f74c1a516326))
* track the publish package ([14a1b60](https://github.com/dsswift/release-damnit/commit/14a1b603fc068dbc06836fe36f29a97b6081d144))

## [1.3.0](https://github.com/dsswift/release-damnit/compare/v1.2.0...v1.3.0) (2026-08-17)


### Features

* add GitLab release provider ([2f0b770](https://github.com/dsswift/release-damnit/commit/2f0b7706f8f9f896ae9c543e08a8ec86e38a2037))


### Bug Fixes

* enforce gitlab release api contracts ([94f41e5](https://github.com/dsswift/release-damnit/commit/94f41e539e3f20f790c1eefa570820de19069c05))

## [1.2.0](https://github.com/dsswift/release-damnit/compare/v1.1.0...v1.2.0) (2026-02-03)


### Features

* trigger patch bump for chore, refactor, style, test, build, ci ([f1c059e](https://github.com/dsswift/release-damnit/commit/f1c059e52f8786a4b70b05243121e7b053fbf44c))

## [1.1.0](https://github.com/dsswift/release-damnit/compare/v1.0.0...v1.1.0) (2026-02-03)


### Features

* add verbose flag for detailed analysis output ([6f933d0](https://github.com/dsswift/release-damnit/commit/6f933d0d3f013ddb3a35d9ba082c5477a22f0873))

## 1.0.0 (2026-02-01)


### Features

* add json release_report and analysis_input outputs ([d901e60](https://github.com/dsswift/release-damnit/commit/d901e6083c9237db3f54ca8c532b2e5714d4fe09))
* complete phase 4 with integration and E2E tests ([657f954](https://github.com/dsswift/release-damnit/commit/657f95458791275d27295527e7e3a9cab51506c2))
* complete phase 5 with GitHub release integration ([90b0556](https://github.com/dsswift/release-damnit/commit/90b0556cab42fa6f487d9a84cd07c4439efb6216))
* implement core release-damnit functionality ([831b33d](https://github.com/dsswift/release-damnit/commit/831b33d4e7a5181dcfb78491da3c9fdd025b90cb))
