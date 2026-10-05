// Package library indexes a music collection on disk (the LIBRARY_DIR
// tree plus the download folder) and answers "is there a file that looks
// like this track?" queries before a download starts. It knows nothing
// about the UI or the network — only about files, their tags and their
// normalized names.
package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"yamdl/internal/norm"
)

// cacheVersion marks the on-disk schema; an unknown version is ignored
// and the index is rebuilt from scratch.
const cacheVersion = 1

// Entry is one indexed audio file.
type Entry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	MTime      int64  `json:"mtime"` // UnixNano
	Ext        string `json:"ext"`   // ".flac"
	Artists    string `json:"artists"`
	Title      string `json:"title"`
	TitleKey   string `json:"title_key"`   // normalized for comparison
	ArtistsKey string `json:"artists_key"` // order-independent key
	Matchable  bool   `json:"matchable"`   // has a usable title
}

type cacheDoc struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// fileMeta is the stat result used to detect changed files cheaply.
type fileMeta struct {
	size  int64
	mtime int64
	ext   string
}

// Index is a thread-safe in-memory index with an optional disk cache.
type Index struct {
	mu        sync.Mutex
	entries   map[string]Entry
	cachePath string
	roots     []string

	scanning bool
	scanned  int
	total    int
}

// New builds an index over roots and loads the disk cache if present.
// Loading is synchronous (a JSON read is cheap); the expensive scan is
// Refresh, meant to run in the background.
func New(cachePath string, roots ...string) *Index {
	ix := &Index{entries: map[string]Entry{}, cachePath: cachePath}
	for _, r := range roots {
		if r = strings.TrimSpace(r); r != "" {
			ix.roots = append(ix.roots, r)
		}
	}
	ix.load()
	return ix
}

func (ix *Index) load() {
	if ix.cachePath == "" {
		return
	}
	raw, err := os.ReadFile(ix.cachePath)
	if err != nil {
		return
	}
	var doc cacheDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != cacheVersion {
		return // unknown/broken cache → full rescan
	}
	for _, e := range doc.Entries {
		if e.Path != "" {
			ix.entries[e.Path] = e
		}
	}
}

// save writes the cache atomically (temp → rename), the project's rule
// for publishing files.
func (ix *Index) save() {
	if ix.cachePath == "" {
		return
	}
	ix.mu.Lock()
	doc := cacheDoc{Version: cacheVersion, Entries: make([]Entry, 0, len(ix.entries))}
	for _, e := range ix.entries {
		doc.Entries = append(doc.Entries, e)
	}
	ix.mu.Unlock()
	sort.Slice(doc.Entries, func(i, j int) bool { return doc.Entries[i].Path < doc.Entries[j].Path })

	raw, err := json.Marshal(doc)
	if err != nil {
		return
	}
	tmp := ix.cachePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, ix.cachePath); err != nil {
		os.Remove(tmp)
	}
}

// collect walks every root and stats the audio files. Non-audio files
// and unreadable directories are skipped; duplicate paths (nested roots)
// collapse into one entry.
func (ix *Index) collect() map[string]fileMeta {
	out := map[string]fileMeta{}
	for _, root := range ix.roots {
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if !audioExts[ext] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			out[path] = fileMeta{size: info.Size(), mtime: info.ModTime().UnixNano(), ext: ext}
			return nil
		})
	}
	return out
}

// Refresh rescans all roots, re-reads tags only for new or changed files,
// drops deleted ones and rewrites the cache. log receives the progress
// lines ("[index] сканирование коллекции ..."). Safe to call from a
// background goroutine; FindMatches keeps answering meanwhile.
func (ix *Index) Refresh(log func(string)) {
	if log == nil {
		log = func(string) {}
	}
	files := ix.collect()

	// Diff against the current state under the lock.
	var toRead []string
	ix.mu.Lock()
	for path, fm := range files {
		old, ok := ix.entries[path]
		if !ok || old.Size != fm.size || old.MTime != fm.mtime || old.Ext != fm.ext {
			toRead = append(toRead, path)
		}
	}
	for path := range ix.entries {
		if _, ok := files[path]; !ok {
			delete(ix.entries, path)
		}
	}
	base := len(files) - len(toRead)
	ix.total = len(files)
	ix.scanned = base
	ix.scanning = true
	ix.mu.Unlock()
	sort.Strings(toRead)

	roots := strings.Join(ix.roots, "; ")
	log(fmtIndexLine(roots, base, len(files)))

	start := time.Now()
	step := len(toRead) / 10
	if step < 1 {
		step = 1
	}
	read := 0
	for i, path := range toRead {
		fm := files[path]
		e := readEntry(path, fm)
		ix.mu.Lock()
		ix.entries[e.Path] = e
		ix.scanned = base + i + 1
		ix.mu.Unlock()
		read++
		if (i+1)%step == 0 && i+1 < len(toRead) {
			log(fmtIndexLine(roots, base+i+1, len(files)))
		}
	}

	ix.save()
	ix.mu.Lock()
	ix.scanning = false
	ix.scanned = ix.total
	ix.mu.Unlock()

	elapsed := time.Since(start).Round(100 * time.Millisecond)
	if read == 0 {
		log(fmt.Sprintf("[index] кэш актуален: %d файлов, без изменений", len(files)))
		return
	}
	log(fmt.Sprintf("[index] индекс готов: %d файлов, перечитано %d, %s",
		len(files), read, elapsed))
}

func fmtIndexLine(roots string, done, total int) string {
	return fmt.Sprintf("[index] сканирование коллекции %s: %d/%d файлов...", roots, done, total)
}

// readEntry builds one Entry: tags first, file name as the fallback.
func readEntry(path string, fm fileMeta) Entry {
	e := Entry{
		Path: path, Size: fm.size, MTime: fm.mtime, Ext: fm.ext,
	}
	artists, title := readTags(path, fm.ext)
	if strings.TrimSpace(title) == "" {
		artists, title = fromFilename(filepath.Base(path))
	}
	e.Artists = strings.Join(artists, ", ")
	e.Title = title
	e.TitleKey = norm.Comparable(title)
	e.ArtistsKey = norm.ArtistsKey(artists)
	e.Matchable = e.TitleKey != ""
	return e
}

// AddFile indexes one freshly published file without rescanning, so a
// download is immediately visible to the next duplicate check.
func (ix *Index) AddFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if !audioExts[ext] {
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return
	}
	e := readEntry(path, fileMeta{size: st.Size(), mtime: st.ModTime().UnixNano(), ext: ext})
	ix.mu.Lock()
	ix.entries[e.Path] = e
	ix.mu.Unlock()
}

// Scanning reports whether a background refresh is still running.
func (ix *Index) Scanning() bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.scanning
}

// Progress is the (processed, total) file counter of the current scan.
func (ix *Index) Progress() (int, int) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.scanned, ix.total
}

// Len is the number of indexed files (test/observability helper).
func (ix *Index) Len() int {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return len(ix.entries)
}
