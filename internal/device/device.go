package device

import (
	"strings"

	"reManager/internal/component"
)

func DetectDevice(machineOutput string) component.DeviceType {
	machine := strings.TrimSpace(machineOutput)
	switch {
	case strings.Contains(machine, "reMarkable 1"):
		return component.DeviceRM1
	case strings.Contains(machine, "reMarkable 2"):
		return component.DeviceRM2
	case strings.Contains(machine, "Ferrari"):
		return component.DeviceRMPP
	case strings.Contains(machine, "Chiappa"):
		return component.DeviceRMPPMove
	default:
		return component.DeviceType(machine)
	}
}

func GetArchitecture(device component.DeviceType) component.DeviceArchitecture {
	switch device {
	case component.DeviceRMPP, component.DeviceRMPPMove:
		return component.ArchAarch64
	default:
		return component.ArchArm32
	}
}

func GetDisplayName(machine string) string {
	machine = strings.TrimSpace(machine)
	switch {
	case strings.Contains(machine, "reMarkable 1"):
		return "reMarkable 1"
	case strings.Contains(machine, "reMarkable 2"):
		return "reMarkable 2"
	case strings.Contains(machine, "Ferrari"):
		return "reMarkable Paper Pro"
	case strings.Contains(machine, "Chiappa"):
		return "reMarkable Paper Pro (Move)"
	default:
		return machine
	}
}
