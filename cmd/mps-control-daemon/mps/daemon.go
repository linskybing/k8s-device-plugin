/**
# Copyright 2024 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package mps

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencontainers/selinux/go-selinux"
	"k8s.io/klog/v2"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
	"github.com/NVIDIA/k8s-device-plugin/internal/rm"
)

type computeMode string

const (
	mpsControlBin = "nvidia-cuda-mps-control"

	computeModeExclusiveProcess = computeMode("EXCLUSIVE_PROCESS")
	computeModeDefault          = computeMode("DEFAULT")

	unprivilegedContainerSELinuxLabel = "system_u:object_r:container_file_t:s0"
)

// Daemon represents an MPS daemon.
// It is associated with a specific kubernets resource and is responsible for
// starting and stopping the deamon as well as ensuring that the memory and
// thread limits are set for the devices that the resource makes available.
type Daemon struct {
	rm rm.ResourceManager
	// root represents the root at which the files and folders controlled by the
	// daemon are created. These include the log and pipe directories.
	root Root
	// logTailer tails the MPS control daemon logs.
	logTailer *tailer
	// mpsConfig carries per-GPU MPS tuning (thread limits, pinned memory)
	mpsConfig *spec.GPUMPSConfig
}

// NewDaemon creates an MPS daemon instance.
func NewDaemon(rm rm.ResourceManager, root Root, cfg *spec.GPUMPSConfig) *Daemon {
	return &Daemon{
		rm:        rm,
		root:      root,
		mpsConfig: cfg,
	}
}

// Devices returns the list of devices under the control of this MPS daemon.
func (d *Daemon) Devices() rm.Devices {
	return d.rm.Devices()
}

type envvars map[string]string

func (e envvars) toSlice() []string {
	var envs []string
	for k, v := range e {
		envs = append(envs, k+"="+v)
	}
	return envs
}

// EnvVars returns the environment variables required for the daemon.
// These should be passed to clients consuming the device shared using MPS.
// TODO: Set CUDA_VISIBLE_DEVICES to include only the devices for this resource type.
func (d *Daemon) EnvVars() envvars {
	env := map[string]string{
		"CUDA_MPS_PIPE_DIRECTORY": d.PipeDir(),
		"CUDA_MPS_LOG_DIRECTORY":  d.LogDir(),
	}
	// Scope the server to only the devices managed by this resource; otherwise
	// server may try device 0 even when serving gpu-1..n, leading to busy/unavailable.
	if cvd := d.VisibleDevices(); cvd != "" {
		env["CUDA_VISIBLE_DEVICES"] = cvd
	}
	// Do NOT inject CUDA_MPS_ACTIVE_THREAD_PERCENTAGE or CUDA_MPS_ACTIVE_THREAD_LIMIT
	// Let each pod specify its own values in the pod manifest.
	return env
}

// VisibleDevices returns a comma-separated CUDA_VISIBLE_DEVICES value scoped to this resource.
func (d *Daemon) VisibleDevices() string {
	// For replicated devices, we need unique GPU indices
	seen := make(map[string]bool)
	var ids []string
	for _, dev := range d.Devices() {
		// Use device index; this aligns with non-MIG per-GPU resources.
		if !seen[dev.Index] {
			ids = append(ids, dev.Index)
			seen[dev.Index] = true
		}
	}
	return strings.Join(ids, ",")
}

// Start starts the MPS deamon as a background process.
func (d *Daemon) Start() error {
	if err := d.setComputeMode(computeModeExclusiveProcess); err != nil {
		return fmt.Errorf("error setting compute mode %v: %w", computeModeExclusiveProcess, err)
	}

	klog.InfoS("Staring MPS daemon", "resource", d.rm.Resource())

	pipeDir := d.PipeDir()
	if err := os.MkdirAll(pipeDir, 0755); err != nil {
		return fmt.Errorf("error creating directory %v: %w", pipeDir, err)
	}

	if err := setSELinuxContext(pipeDir, unprivilegedContainerSELinuxLabel); err != nil {
		return fmt.Errorf("error setting SELinux context: %w", err)
	}

	logDir := d.LogDir()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("error creating directory %v: %w", logDir, err)
	}

	mpsDaemon := exec.Command(mpsControlBin, "-d")
	mpsDaemon.Env = append(mpsDaemon.Env, d.EnvVars().toSlice()...)
	
	// Capture stderr/stdout to help diagnose failures
	var stderr, stdout bytes.Buffer
	mpsDaemon.Stderr = &stderr
	mpsDaemon.Stdout = &stdout
	
	if err := mpsDaemon.Run(); err != nil {
		klog.ErrorS(err, "Failed to start MPS daemon",
			"resource", d.rm.Resource(),
			"env", d.EnvVars(),
			"stderr", stderr.String(),
			"stdout", stdout.String())
		return fmt.Errorf("exit code %v: %s", err, stderr.String())
	}
	
	klog.InfoS("MPS daemon command completed",
		"resource", d.rm.Resource(),
		"stdout", stdout.String(),
		"stderr", stderr.String())

	for index, limit := range d.perDevicePinnedDeviceMemoryLimits() {
		_, err := d.EchoPipeToControl(fmt.Sprintf("set_default_device_pinned_mem_limit %s %s", index, limit))
		if err != nil {
			return fmt.Errorf("error setting pinned memory limit for device %v: %w", index, err)
		}
	}
	// Do NOT set default active thread percentage at server side.
	// Let each client pod control its own thread percentage via CUDA_MPS_ACTIVE_THREAD_PERCENTAGE env var.
	klog.InfoS("MPS daemon started without default thread percentage; clients control their own limits",
		"resource", d.rm.Resource())

	statusFile, err := os.Create(d.startedFile())
	if err != nil {
		return err
	}
	defer statusFile.Close()

	d.logTailer = newTailer(filepath.Join(logDir, "control.log"))
	klog.InfoS("Starting log tailer", "resource", d.rm.Resource())
	if err := d.logTailer.Start(); err != nil {
		klog.ErrorS(err, "Could not start tail command on control.log; ignoring logs")
	}

	return nil
}

func setSELinuxContext(path string, context string) error {
	_, err := os.Stat("/sys/fs/selinux")
	if err != nil && errors.Is(err, os.ErrNotExist) {
		klog.InfoS("SELinux disabled, not updating context", "path", path)
		return nil
	} else if err != nil {
		return fmt.Errorf("error checking if SELinux is enabled: %w", err)
	}

	klog.InfoS("SELinux enabled, setting context", "path", path, "context", context)
	return selinux.Chcon(path, context, true)
}

// Stop ensures that the MPS daemon is quit.
func (d *Daemon) Stop() error {
	_, err := d.EchoPipeToControl("quit")
	if err != nil {
		return fmt.Errorf("error sending quit message: %w", err)
	}
	klog.InfoS("Stopped MPS control daemon", "resource", d.rm.Resource())

	var tailErr error
	if d.logTailer != nil {
		tailErr = d.logTailer.Stop()
	}
	klog.InfoS("Stopped log tailer", "resource", d.rm.Resource(), "error", tailErr)

	if err := d.setComputeMode(computeModeDefault); err != nil {
		return fmt.Errorf("error setting compute mode %v: %w", computeModeDefault, err)
	}

	if err := os.Remove(d.startedFile()); err != nil && err != os.ErrNotExist {
		return fmt.Errorf("failed to remove started file: %w", err)
	}

	logDir := d.LogDir()
	if err := os.RemoveAll(logDir); err != nil {
		klog.ErrorS(err, "Failed to remove pipe directory", "path", logDir)
	}

	return nil
}

func (d *Daemon) LogDir() string {
	return d.root.LogDir(d.rm.Resource())
}

func (d *Daemon) PipeDir() string {
	return d.root.PipeDir(d.rm.Resource())
}

func (d *Daemon) ShmDir() string {
	return "/dev/shm"
}

func (d *Daemon) startedFile() string {
	return d.root.startedFile(d.rm.Resource())
}

// AssertHealthy checks that the MPS control daemon is healthy.
func (d *Daemon) AssertHealthy() error {
	_, err := d.EchoPipeToControl("get_default_active_thread_percentage")
	return err
}

// EchoPipeToControl sends the specified command to the MPS control daemon.
func (d *Daemon) EchoPipeToControl(command string) (string, error) {
	var out bytes.Buffer
	reader, writer := io.Pipe()
	defer writer.Close()
	defer reader.Close()

	mpsDaemon := exec.Command(mpsControlBin)
	mpsDaemon.Env = append(mpsDaemon.Env, d.EnvVars().toSlice()...)

	mpsDaemon.Stdin = reader
	mpsDaemon.Stdout = &out

	if err := mpsDaemon.Start(); err != nil {
		return "", fmt.Errorf("failed to start NVIDIA MPS command: %w", err)
	}

	if _, err := writer.Write([]byte(command)); err != nil {
		return "", fmt.Errorf("failed to write message to pipe: %w", err)
	}
	_ = writer.Close()

	if err := mpsDaemon.Wait(); err != nil {
		return "", fmt.Errorf("failed to send command to MPS daemon: %w", err)
	}
	return out.String(), nil
}

func (d *Daemon) setComputeMode(mode computeMode) error {
	for _, uuid := range d.Devices().GetUUIDs() {
		cmd := exec.Command(
			"nvidia-smi",
			"-i", uuid,
			"-c", string(mode))
		output, err := cmd.CombinedOutput()
		if err != nil {
			klog.Errorf("\n%v", string(output))
			return fmt.Errorf("error running nvidia-smi: %w", err)
		}
	}
	return nil
}

// perDevicePinnedMemoryLimits returns the pinned memory limits for each device.
func (m *Daemon) perDevicePinnedDeviceMemoryLimits() map[string]string {
	totalMemoryInBytesPerDevice := make(map[string]uint64)
	replicasPerDevice := make(map[string]uint64)
	for _, device := range m.Devices() {
		index := device.Index
		totalMemoryInBytesPerDevice[index] = device.TotalMemory
		replicasPerDevice[index] += 1
	}

	limits := make(map[string]string)
	for index, totalMemory := range totalMemoryInBytesPerDevice {
		if totalMemory == 0 {
			continue
		}
		// If a pinned memory limit is explicitly set in config, apply it per device
		if m.mpsConfig != nil && m.mpsConfig.PinnedMemoryLimit > 0 {
			limits[index] = fmt.Sprintf("%dM", m.mpsConfig.PinnedMemoryLimit)
			continue
		}

		replicas := replicasPerDevice[index]
		limits[index] = fmt.Sprintf("%vM", totalMemory/replicas/1024/1024)
	}
	return limits
}

func (m *Daemon) activeThreadPercentage() string {
	if len(m.Devices()) == 0 {
		return ""
	}
	// Explicit config takes priority
	if m.mpsConfig != nil && m.mpsConfig.ActiveThreadPercentage > 0 {
		return fmt.Sprintf("%d", m.mpsConfig.ActiveThreadPercentage)
	}
	// Auto-calculate from replicas: find the minimum replicas across all devices
	// and use 100/replicas as the thread percentage
	minReplicas := uint64(0)
	replicasPerDevice := make(map[string]uint64)
	for _, device := range m.Devices() {
		replicasPerDevice[device.Index] += 1
	}
	for _, replicas := range replicasPerDevice {
		if minReplicas == 0 || replicas < minReplicas {
			minReplicas = replicas
		}
	}
	if minReplicas > 1 {
		percentage := 100 / minReplicas
		klog.InfoS("Auto-calculated active thread percentage from replicas", 
			"resource", m.rm.Resource(), "replicas", minReplicas, "percentage", percentage)
		return fmt.Sprintf("%d", percentage)
	}
	// Single replica or no replicas: no thread limit
	return ""
}
