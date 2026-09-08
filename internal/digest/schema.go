package digest

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

const SchemaVersion = 1

type MapEntry struct {
	Lines string `json:"lines"`
	Kind  string `json:"kind"`
}
type FileMap struct {
	Summary string     `json:"summary"`
	Map     []MapEntry `json:"map"`
	Symbols []string   `json:"symbols"`
	Notes   string     `json:"notes"`
}

type Cluster struct {
	Where   string `json:"where"`
	Matches int    `json:"matches"`
	Gist    string `json:"gist"`
}
type Clusters struct {
	Summary             string    `json:"summary"`
	Clusters            []Cluster `json:"clusters"`
	Total               int       `json:"total"`
	RepresentativeFiles []string  `json:"representative_files"`
}

type Run struct {
	Summary  string   `json:"summary"`
	KeyLines []string `json:"key_lines"`
	ExitCode int      `json:"exit_code"`
	LogPath  string   `json:"log_path"`
}

var lineRange = regexp.MustCompile(`^\d+(-\d+)?$`)

func ParseFileMap(b []byte) (FileMap, error) {
	var fm FileMap
	if err := json.Unmarshal(b, &fm); err != nil {
		return FileMap{}, err
	}
	if fm.Summary == "" {
		return FileMap{}, errors.New("filemap: empty summary")
	}
	if len(fm.Map) == 0 {
		return FileMap{}, errors.New("filemap: empty map")
	}
	for _, m := range fm.Map {
		if !lineRange.MatchString(m.Lines) {
			return FileMap{}, fmt.Errorf("filemap: bad line range %q", m.Lines)
		}
	}
	return fm, nil
}

func ParseClusters(b []byte) (Clusters, error) {
	var c Clusters
	if err := json.Unmarshal(b, &c); err != nil {
		return Clusters{}, err
	}
	if c.Summary == "" {
		return Clusters{}, errors.New("clusters: empty summary")
	}
	if len(c.Clusters) == 0 {
		return Clusters{}, errors.New("clusters: empty clusters")
	}
	return c, nil
}

func ParseRun(b []byte) (Run, error) {
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return Run{}, err
	}
	if r.Summary == "" {
		return Run{}, errors.New("run: empty summary")
	}
	return r, nil
}
