Mise — Development toolchain (quickstart)

This repository uses Mise to provide consistent developer toolchains for Go, Python, and
Node.js. Mise ensures contributors run the same tool versions as CI and the Docker image.

Prerequisites
- Install mise: https://github.com/mise-dev/mise (follow platform instructions)

Quick Mise usage

This repository uses Mise to provide consistent developer toolchains for Go, Python, and
Node.js. Mise ensures contributors run the same tool versions as CI and the Docker image.

Prerequisites
- Install mise: https://github.com/mise-dev/mise (follow platform instructions)

Notes
- The file mise.toml at the repository root defines which tools are managed. Some Mise
  versions do not accept an 'aliases' table in the root mise.toml. To avoid parse errors,
  you can copy the alias examples below into your local mise.toml if your Mise supports
  them.

Further reading
- https://github.com/mise-dev/mise
