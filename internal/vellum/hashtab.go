package vellum

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"reManager/internal/executor"
)

const (
	HashtabPath         = "/home/root/xovi/exthome/qt-resource-rebuilder/hashtab"
	HashtabVersionMagic = 17607111715072197239
)

type HashtabChecker struct {
	executor executor.CommandExecutor
}

func NewHashtabChecker(exec executor.CommandExecutor) *HashtabChecker {
	return &HashtabChecker{executor: exec}
}

func (h *HashtabChecker) CheckHashtabExists() (bool, error) {
	cmd := fmt.Sprintf("test -f %s && echo yes || echo no", HashtabPath)
	output, err := h.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) == "yes", nil
}

func (h *HashtabChecker) GetHashtabVersion() (string, error) {
	cmd := fmt.Sprintf("xxd -l 128 -p %s", HashtabPath)
	output, err := h.executor.ExecuteWithOutput(cmd)
	if err != nil {
		return "", err
	}
	return parseHashtabVersion(output)
}

func parseHashtabVersion(hexData string) (string, error) {
	hexData = strings.ReplaceAll(hexData, "\n", "")
	hexData = strings.ReplaceAll(hexData, " ", "")

	if len(hexData) < 24 {
		return "", fmt.Errorf("hashtab too short")
	}

	offset := 0

	hashBytes, err := hex.DecodeString(hexData[offset : offset+16])
	if err != nil {
		return "", err
	}
	firstHash := binary.BigEndian.Uint64(hashBytes)
	offset += 16

	lenBytes, err := hex.DecodeString(hexData[offset : offset+8])
	if err != nil {
		return "", err
	}
	firstStrLen := binary.BigEndian.Uint32(lenBytes)
	offset += 8

	offset += int(firstStrLen) * 2

	if firstHash != 0 {
		return "", fmt.Errorf("unexpected first entry hash")
	}

	if len(hexData) < offset+24 {
		return "", nil
	}

	hashBytes, err = hex.DecodeString(hexData[offset : offset+16])
	if err != nil {
		return "", err
	}
	secondHash := binary.BigEndian.Uint64(hashBytes)
	offset += 16

	if secondHash != HashtabVersionMagic {
		return "", nil
	}

	lenBytes, err = hex.DecodeString(hexData[offset : offset+8])
	if err != nil {
		return "", err
	}
	versionLen := binary.BigEndian.Uint32(lenBytes)
	offset += 8

	endIdx := offset + int(versionLen)*2
	if len(hexData) < endIdx {
		return "", fmt.Errorf("incomplete version string")
	}

	versionBytes, err := hex.DecodeString(hexData[offset:endIdx])
	if err != nil {
		return "", err
	}

	return string(versionBytes), nil
}
