# K8s Device Plugin

The K8s Device Plugin is an enhanced version of the NVIDIA device plugin for Kubernetes. it provides advanced GPU resource management, including sharing strategies and fine-grained resource allocation.

## Key Features

- **GPU Sharing**: Multiple containers can share the same physical GPU using NVIDIA MPS or Time-Slicing.
- **Milli-GPU Support**: Expose a single GPU as multiple logical resources (replicated resources) to allow fractional GPU allocation.
- **Dynamic Configuration**: Configure sharing strategies and resource renaming via a configuration file or Helm values.
- **Health Monitoring**: Continuously monitors the health of GPU devices and reports them to Kubernetes.

## Components

- **nvidia-device-plugin**: The main component that registers as a Kubernetes Device Plugin.
- **mps-control-daemon**: Manages the NVIDIA MPS server on nodes where MPS sharing is enabled.
- **gpu-feature-discovery**: Automatically labels nodes with GPU capabilities and configurations.

## Getting Started

To get started with the K8s Device Plugin, please refer to the [Installation](installation.md) guide and the [Configuration](configuration.md) details.
