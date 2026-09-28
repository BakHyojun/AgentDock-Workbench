package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/uvwt/agentdock/internal/fs/atomicfile"
)

// Bundled plugins ship inside the AgentDock payload. Setup provisions them into
// the user's store and keeps them current, but never overrides the user: a
// plugin installed separately under the same name, or one the user removed
// after Setup provided it, stays as the user left it. Host state such as the
// enabled switch is preserved by the normal replace path.
const bundledRecordFile = ".bundled.json"

const (
	BundledInstalled   = "installed"
	BundledUpdated     = "updated"
	BundledCurrent     = "current"
	BundledUserOwned   = "user_owned"
	BundledUserRemoved = "user_removed"
	BundledFailed      = "failed"
)

type BundledResult struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

type bundledRecord struct {
	SchemaVersion int                     `json:"schema_version"`
	Plugins       map[string]bundledEntry `json:"plugins"`
}

type bundledEntry struct {
	Version string `json:"version"`
}

// ProvisionBundled installs or updates every plugin directory under bundleRoot.
// A missing bundle root is not an error: older payloads carry no plugins.
func (s *Store) ProvisionBundled(bundleRoot string) ([]BundledResult, error) {
	entries, err := os.ReadDir(bundleRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []BundledResult{}, nil
	}
	if err != nil {
		return nil, newError("PLUGIN_BUNDLE_READ_FAILED", "list bundled plugins", map[string]any{"bundle": bundleRoot}, err)
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return nil, newError("PLUGIN_STORE_WRITE_FAILED", "create plugin store", nil, err)
	}
	record, err := s.readBundledRecord()
	if err != nil {
		return nil, err
	}
	installed, err := s.scan()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	results := make([]BundledResult, 0, len(entries))
	var failures []error
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		source := filepath.Join(bundleRoot, entry.Name())
		bundled, err := readPackage(source, false)
		if err != nil {
			results = append(results, BundledResult{Name: entry.Name(), Action: BundledFailed, Error: err.Error()})
			failures = append(failures, err)
			continue
		}
		name, version := bundled.manifest.Name, bundled.manifest.Version
		existing, isInstalled := installed[name]
		_, provided := record.Plugins[name]
		result := BundledResult{Name: name, Version: version}
		switch {
		case isInstalled && !provided:
			result.Action = BundledUserOwned
		case !isInstalled && provided:
			result.Action = BundledUserRemoved
		case isInstalled && existing.manifest.Version == version:
			result.Action = BundledCurrent
		default:
			result.Action = BundledInstalled
			if isInstalled {
				result.Action = BundledUpdated
			}
			if _, err := s.Install(source, isInstalled); err != nil {
				result.Action, result.Error = BundledFailed, err.Error()
				failures = append(failures, err)
			}
		}
		if result.Action == BundledInstalled || result.Action == BundledUpdated || result.Action == BundledCurrent {
			record.Plugins[name] = bundledEntry{Version: version}
		}
		results = append(results, result)
	}
	if err := s.writeBundledRecord(record); err != nil {
		failures = append(failures, err)
	}
	return results, errors.Join(failures...)
}

func (s *Store) readBundledRecord() (bundledRecord, error) {
	record := bundledRecord{SchemaVersion: 1, Plugins: map[string]bundledEntry{}}
	data, err := os.ReadFile(filepath.Join(s.root, bundledRecordFile))
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return record, newError("PLUGIN_STORE_READ_FAILED", "read bundled plugin record", nil, err)
	}
	if err := json.Unmarshal(data, &record); err != nil || record.SchemaVersion != 1 {
		return record, newError("PLUGIN_STATE_INVALID", "bundled plugin record is invalid", nil, err)
	}
	if record.Plugins == nil {
		record.Plugins = map[string]bundledEntry{}
	}
	return record, nil
}

func (s *Store) writeBundledRecord(record bundledRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.Write(filepath.Join(s.root, bundledRecordFile), append(data, '\n'), 0o600); err != nil {
		return newError("PLUGIN_STORE_WRITE_FAILED", "write bundled plugin record", nil, err)
	}
	return nil
}
