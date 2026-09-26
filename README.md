<p align="center">
  <img src="./branding/banner.png" width="550"/>
</p>

File Browser provides a file managing interface within a specified directory and it can be used to upload, delete, preview and edit your files. It is a **create-your-own-cloud**-kind of software where you can just install it on your server, direct it to a path and access your files through a nice web interface.

This is a community-maintained fork that keeps File Browser alive after the original project was archived. Multi-architecture Docker images (`linux/amd64` and `linux/arm64`) are published to [`shounak6942/filebrowser`](https://hub.docker.com/r/shounak6942/filebrowser).

## Quick Start

The fastest way to get started is with Docker:

```sh
docker run \
    -v filebrowser_data:/srv \
    -v filebrowser_database:/database \
    -v filebrowser_config:/config \
    -p 8080:80 \
    shounak6942/filebrowser:latest
```

Then open <http://localhost:8080> and sign in as `admin`. The randomly generated
password is printed in the container logs on first boot. The image runs on both
`amd64` and `arm64` hosts — Docker automatically selects the right one.

Prefer a binary or need more options (volumes, bind mounts, first-boot setup)?
See [`docs/installation.md`](docs/installation.md).

## Security

File Browser is powerful software; run it with care. Two behaviors are worth
understanding before you expose an instance:

- **Command execution, runner, and hooks.** This feature lets users run commands on the host. It is disabled by default; if you re-enable it with `--disable-exec=false`, treat the ability to run commands as equivalent to shell access on the host. Background: [#5199](https://github.com/filebrowser/filebrowser/issues/5199).
- **Session and JWT handling.** Sessions are self-contained JWTs rather than server-side identifiers, so they cannot be revoked, which means that logout, password changes, and renewal leave previously issued tokens valid until they expire, and the same refresh token can be redeemed repeatedly. Assume a leaked token is valid until expiry. Background: [#5216](https://github.com/filebrowser/filebrowser/issues/5216).

Recommended hardening:

- **Put it behind a reverse proxy** that terminates TLS and performs its own authentication before exposing it to the internet.
- **Keep the command runner disabled** unless you fully trust every user. It is off by default, so leave it off. See [`docs/command-execution.md`](docs/command-execution.md).
- **Run it unprivileged, inside a container**, with only the directory you intend to serve mounted into it.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Documentation

Documentation on how to install, configure, and build this project lives in [`docs`](docs) in this repository.

[CONTRIBUTING.md](CONTRIBUTING.md) documents how to build and develop the project.

## License

[Apache License 2.0](LICENSE) © File Browser Contributors
