# Building from Source

To build the K8s Device Plugin from source, you need Go installed.

## Build Requirements

- Go 1.22+
- Docker (for container builds)
- Make

## Build Commands

Build the binary:

```bash
make build
```

The binary will be located in the `bin/` directory.

## Build Docker Image

```bash
make docker-build
```

You can specify the image name and tag:

```bash
IMAGE_NAME=my-registry/nvidia-device-plugin TAG=v1.0.0 make docker-build
```

## Running Tests

Execute the unit tests:

```bash
make test
```
