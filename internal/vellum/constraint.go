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

// Searched level by level across the whole queue, so the entry closest to the package claims it however deep in its tree it sits.
func (m *MetadataStore) queueEntryProviding(name string, queue []string, deviceType, firmware, arch string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	owner := make(map[string]string)
	var frontier []string
	for _, queued := range queue {
		if _, seen := owner[queued]; !seen {
			owner[queued] = queued
			frontier = append(frontier, queued)
		}
	}
	for len(frontier) > 0 {
		if found, ok := owner[name]; ok {
			return found
		}
		var next []string
		for _, pkg := range frontier {
			_, info := m.bestVersion(pkg, deviceType, firmware, arch)
			if info == nil {
				continue
			}
			for _, dep := range info.Depends {
				if strings.HasPrefix(dep, "!") {
					continue
				}
				depName := ParseConstraint(dep).Name
				if _, seen := owner[depName]; seen {
					continue
				}
				owner[depName] = owner[pkg]
				next = append(next, depName)
			}
		}
		frontier = next
	}
	return owner[name]
}

type presentPackage struct {
	version   string
	conflicts []string
	queued    bool
	// The installed or queued package this one is here for; a conflict with it is reported against this.
	root string
}

type treePackage struct {
	name      string
	version   string
	conflicts []string
}

// Follows real dependencies only: a virtual one is either already satisfied or becomes a question before anything installs.
func (m *MetadataStore) dependencyTree(root string, deviceType, firmware, arch string) []treePackage {
	var tree []treePackage
	visited := map[string]bool{}
	queue := []string{root}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		version, info := m.bestVersion(name, deviceType, firmware, arch)
		if info == nil {
			continue
		}
		tree = append(tree, treePackage{name: name, version: version, conflicts: info.Conflicts})
		for _, dep := range info.Depends {
			if strings.HasPrefix(dep, "!") {
				continue
			}
			queue = append(queue, ParseConstraint(dep).Name)
		}
	}
	return tree
}

// Checks each candidate's whole dependency tree against everything present. Versions follow bestVersion, which approximates apk's choice; the plan's solve stays the authority.
func (m *MetadataStore) FindConflicts(installed map[string]string, queue []string, deviceType, firmware, arch string) map[string]PackageConflict {
	m.mu.RLock()
	defer m.mu.RUnlock()

	present := make(map[string]presentPackage, len(installed)+len(queue))
	for name, version := range installed {
		entry := presentPackage{version: version, root: name}
		if info, ok := m.packages.Packages[name][version]; ok {
			entry.conflicts = info.Conflicts
		}
		present[name] = entry
	}
	for _, queued := range queue {
		for _, pkg := range m.dependencyTree(queued, deviceType, firmware, arch) {
			if _, already := present[pkg.name]; already {
				continue
			}
			present[pkg.name] = presentPackage{version: pkg.version, conflicts: pkg.conflicts, queued: true, root: queued}
		}
	}

	conflicts := make(map[string]PackageConflict)
	for candidate := range m.packages.Packages {
		if _, alreadyPresent := present[candidate]; alreadyPresent {
			continue
		}
		if conflict, found := m.treeConflict(m.dependencyTree(candidate, deviceType, firmware, arch), present); found {
			conflicts[candidate] = conflict
		}
	}
	return conflicts
}

func (m *MetadataStore) treeConflict(tree []treePackage, present map[string]presentPackage) (PackageConflict, bool) {
	incoming := make(map[string]string, len(tree))
	for _, pkg := range tree {
		if _, shared := present[pkg.name]; !shared {
			incoming[pkg.name] = pkg.version
		}
	}

	for _, pkg := range tree {
		if _, shared := present[pkg.name]; shared {
			continue
		}
		for _, entry := range pkg.conflicts {
			constraint := ParseConstraint(entry)
			other, isPresent := present[constraint.Name]
			if isPresent && constraint.Prohibits(other.version) {
				return PackageConflict{ConflictsWith: other.root, Queued: other.queued}, true
			}
		}
	}

	for _, other := range present {
		for _, entry := range other.conflicts {
			constraint := ParseConstraint(entry)
			version, isIncoming := incoming[constraint.Name]
			if isIncoming && constraint.Prohibits(version) {
				return PackageConflict{ConflictsWith: other.root, Queued: other.queued}, true
			}
		}
	}
	return PackageConflict{}, false
}
