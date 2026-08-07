package vellum

import (
	"strings"

	versionpkg "reManager/internal/version"
)

type VersionConstraint struct {
	Name    string
	Op      string
	Version string
}

var constraintOps = []string{">=", "<=", "=", ">", "<"}

func ParseConstraint(entry string) VersionConstraint {
	for _, op := range constraintOps {
		if i := strings.Index(entry, op); i > 0 {
			return VersionConstraint{
				Name:    entry[:i],
				Op:      op,
				Version: entry[i+len(op):],
			}
		}
	}
	return VersionConstraint{Name: entry}
}

func (c VersionConstraint) Prohibits(version string) bool {
	if c.Op == "" {
		return true
	}
	if version == "" || c.Version == "" {
		return false
	}

	cmp := versionpkg.Compare(version, c.Version)
	switch c.Op {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case "=":
		return cmp == 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	}
	return false
}

func stripDepVersion(dep string) string {
	return ParseConstraint(dep).Name
}

type PackageConflict struct {
	ConflictsWith string `json:"conflictsWith"`
	Queued        bool   `json:"queued"`
}

type QueueConflictEntry struct {
	QueueEntry string `json:"queueEntry"`
	Package    string `json:"package"`
	Blocks     string `json:"blocks"`
}

func (m *MetadataStore) AttributeConflictsToQueue(conflicts []ResolutionConflict, queue []string, deviceType, firmware, arch string) []QueueConflictEntry {
	var entries []QueueConflictEntry
	claimed := make(map[string]bool)

	for _, conflict := range conflicts {
		for _, participant := range []struct{ name, blocks string }{
			{conflict.Package, conflict.Breaks},
			{conflict.Breaks, conflict.Package},
		} {
			owner := m.queueEntryProviding(participant.name, queue, deviceType, firmware, arch)
			if owner == "" || claimed[owner] {
				continue
			}
			claimed[owner] = true
			entries = append(entries, QueueConflictEntry{
				QueueEntry: owner,
				Package:    participant.name,
				Blocks:     participant.blocks,
			})
		}
	}
	return entries
}

func (m *MetadataStore) queueEntryProviding(name string, queue []string, deviceType, firmware, arch string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, queued := range queue {
		if queued == name {
			return queued
		}
	}
	for _, queued := range queue {
		_, info := m.bestVersion(queued, deviceType, firmware, arch)
		if info == nil {
			continue
		}
		for _, dep := range info.Depends {
			if ParseConstraint(dep).Name == name {
				return queued
			}
		}
	}
	return ""
}

type presentPackage struct {
	name      string
	version   string
	conflicts []string
	queued    bool
}

func (m *MetadataStore) FindConflicts(installed map[string]string, queue []string, deviceType, firmware, arch string) map[string]PackageConflict {
	m.mu.RLock()
	defer m.mu.RUnlock()

	present := make([]presentPackage, 0, len(installed)+len(queue))
	presentVersions := make(map[string]string, len(installed)+len(queue))
	presentQueued := make(map[string]bool, len(queue))

	for name, version := range installed {
		entry := presentPackage{name: name, version: version}
		if info, ok := m.packages.Packages[name][version]; ok {
			entry.conflicts = info.Conflicts
		}
		present = append(present, entry)
		presentVersions[name] = version
	}
	for _, name := range queue {
		version, info := m.bestVersion(name, deviceType, firmware, arch)
		entry := presentPackage{name: name, version: version, queued: true}
		if info != nil {
			entry.conflicts = info.Conflicts
		}
		present = append(present, entry)
		presentVersions[name] = version
		presentQueued[name] = true
	}

	conflicts := make(map[string]PackageConflict)

	for candidate := range m.packages.Packages {
		if _, alreadyPresent := presentVersions[candidate]; alreadyPresent {
			continue
		}

		candidateVersion, candidateInfo := m.bestVersion(candidate, deviceType, firmware, arch)
		if candidateInfo == nil {
			continue
		}

		for _, entry := range candidateInfo.Conflicts {
			constraint := ParseConstraint(entry)
			version, isPresent := presentVersions[constraint.Name]
			if isPresent && constraint.Prohibits(version) {
				conflicts[candidate] = PackageConflict{
					ConflictsWith: constraint.Name,
					Queued:        presentQueued[constraint.Name],
				}
				break
			}
		}
		if _, found := conflicts[candidate]; found {
			continue
		}

		for _, pkg := range present {
			for _, entry := range pkg.conflicts {
				constraint := ParseConstraint(entry)
				if constraint.Name == candidate && constraint.Prohibits(candidateVersion) {
					conflicts[candidate] = PackageConflict{
						ConflictsWith: pkg.name,
						Queued:        pkg.queued,
					}
					break
				}
			}
			if _, found := conflicts[candidate]; found {
				break
			}
		}
	}

	return conflicts
}
