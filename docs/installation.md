# Installation Guide

The recommended way to install the K8s Device Plugin is via Helm.

## Prerequisites

- Kubernetes cluster (v1.24+)
- NVIDIA drivers installed on nodes
- Helm 3 installed

## Step 1: Add the Repository

```bash
helm repo add nvhpc https://nvidia.github.io/k8s-device-plugin
helm repo update
```

## Step 2: Configure and Install

Create a `values.yaml` file to enable sharing features:

```yaml
config:
  map:
    default: |-
      version: v1
      sharing:
        timeSlicing:
          resources:
          - name: nvidia.com/gpu
            replicas: 10
```

Install the chart:

```bash
helm install nvidia-device-plugin nvhpc/nvidia-device-plugin 
  --namespace kube-system 
  -f values.yaml
```

## Step 3: Verify Installation

Check if the pods are running:

```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=nvidia-device-plugin
```

Describe a node to see the advertised resources:

```bash
kubectl describe node <node-name> | grep nvidia.com/gpu
```
