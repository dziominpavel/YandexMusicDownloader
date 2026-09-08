package tagger

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/tommyo123/mtag/mp4"
)

// ensureMetaTree creates the moov/udta/meta/ilst path in an otherwise valid
// M4A that ships without iTunes metadata (the usual Yandex case).
// Files that already have ilst are left untouched.
func ensureMetaTree(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("tagger: open m4a: %w", err)
	}
	closed := false
	closeSrc := func() error {
		if closed {
			return nil
		}
		closed = true
		return src.Close()
	}
	defer func() { _ = closeSrc() }()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("tagger: stat m4a: %w", err)
	}
	atoms := mp4.WalkTopLevel(src, info.Size())
	if err := checkTopAtoms(atoms, info.Size()); err != nil {
		return err
	}
	moovIdx := findTopAtom(atoms, "moov")
	if moovIdx < 0 {
		return fmt.Errorf("tagger: m4a has no moov atom")
	}

	old := atoms[moovIdx]
	body := make([]byte, old.DataSize)
	if _, err := src.ReadAt(body, old.DataAt); err != nil {
		return fmt.Errorf("tagger: read moov: %w", err)
	}
	newBody, changed, err := bootstrapMeta(body)
	if err != nil {
		return err
	}
	if !changed {
		return closeSrc()
	}

	newMoov, err := buildAtom("moov", newBody)
	if err != nil {
		return err
	}
	before, after := mdatAroundMoov(atoms, moovIdx)
	if before && after {
		return fmt.Errorf("tagger: unsupported mdat on both sides of moov")
	}
	if after {
		if delta := int64(len(newMoov)) - old.Size; delta != 0 {
			mp4.PatchSampleOffsets(newBody, delta)
			if newMoov, err = buildAtom("moov", newBody); err != nil {
				return err
			}
		}
	}
	return rewriteMoov(src, info, atoms, moovIdx, newMoov, closeSrc)
}

func mdatAroundMoov(atoms []mp4.TopLevelAtom, moovIdx int) (before, after bool) {
	for i, a := range atoms {
		if string(a.Name[:]) != "mdat" {
			continue
		}
		if i < moovIdx {
			before = true
		} else if i > moovIdx {
			after = true
		}
	}
	return before, after
}

func checkTopAtoms(atoms []mp4.TopLevelAtom, fileSize int64) error {
	if fileSize < 8 || len(atoms) == 0 {
		return fmt.Errorf("tagger: malformed mp4 top atoms")
	}
	var cursor int64
	for _, a := range atoms {
		if a.Offset != cursor || a.Size < 8 || a.DataAt < a.Offset || a.DataSize < 0 || a.Offset+a.Size > fileSize {
			return fmt.Errorf("tagger: malformed mp4 top atom")
		}
		cursor += a.Size
	}
	if cursor != fileSize {
		return fmt.Errorf("tagger: malformed mp4 top atoms")
	}
	return nil
}

func findTopAtom(atoms []mp4.TopLevelAtom, name string) int {
	for i, a := range atoms {
		if string(a.Name[:]) == name {
			return i
		}
	}
	return -1
}

// bootstrapMeta inserts udta/meta/ilst as needed. changed=false means ilst exists.
func bootstrapMeta(moov []byte) ([]byte, bool, error) {
	udta, ok, err := findChild(moov, "udta")
	if err != nil {
		return nil, false, fmt.Errorf("tagger: parse moov: %w", err)
	}
	if !ok {
		meta, err := freshMeta()
		if err != nil {
			return nil, false, err
		}
		udtaBody, err := appendAtom(nil, "meta", meta)
		if err != nil {
			return nil, false, err
		}
		out, err := appendAtom(moov, "udta", udtaBody)
		return out, true, err
	}

	udtaBody := moov[udta.off+udta.hdr : udta.end]
	meta, ok, err := findChild(udtaBody, "meta")
	if err != nil {
		return nil, false, fmt.Errorf("tagger: parse udta: %w", err)
	}
	if !ok {
		fresh, err := freshMeta()
		if err != nil {
			return nil, false, err
		}
		newUDTA, err := appendAtom(udtaBody, "meta", fresh)
		if err != nil {
			return nil, false, err
		}
		return replaceAtom(moov, udta, "udta", newUDTA)
	}

	metaBody := udtaBody[meta.off+meta.hdr : meta.end]
	if len(metaBody) < 4 {
		return nil, false, fmt.Errorf("tagger: meta atom too short")
	}
	if _, ok, err := findChild(metaBody[4:], "ilst"); err != nil {
		return nil, false, fmt.Errorf("tagger: parse meta: %w", err)
	} else if ok {
		return moov, false, nil
	}
	children, err := appendAtom(metaBody[4:], "ilst", nil)
	if err != nil {
		return nil, false, err
	}
	newMeta := append(append([]byte{}, metaBody[:4]...), children...)
	newUDTA, _, err := replaceAtom(udtaBody, meta, "meta", newMeta)
	if err != nil {
		return nil, false, err
	}
	return replaceAtom(moov, udta, "udta", newUDTA)
}

// freshMeta builds a minimal meta body: version/flags + hdlr + empty ilst.
func freshMeta() ([]byte, error) {
	hdlr, err := buildAtom("hdlr", []byte{
		0, 0, 0, 0, // version and flags
		0, 0, 0, 0, // pre-defined
		'm', 'd', 'i', 'r', 'a', 'p', 'p', 'l',
		0, 0, 0, 0, // component flags
		0, 0, 0, 0, // component flags mask
		0, // empty component name
	})
	if err != nil {
		return nil, err
	}
	ilst, err := buildAtom("ilst", nil)
	if err != nil {
		return nil, err
	}
	out := []byte{0, 0, 0, 0}
	out = append(out, hdlr...)
	return append(out, ilst...), nil
}

type atomLoc struct {
	off   int
	end   int
	hdr   int
	toEnd bool
	typ   string
}

func findChild(body []byte, target string) (atomLoc, bool, error) {
	for off := 0; off < len(body); {
		a, err := parseAtom(body, off)
		if err != nil {
			return atomLoc{}, false, err
		}
		if a.typ == target {
			return a, true, nil
		}
		off = a.end
	}
	return atomLoc{}, false, nil
}

func parseAtom(data []byte, off int) (atomLoc, error) {
	if off < 0 || len(data)-off < 8 {
		return atomLoc{}, fmt.Errorf("tagger: truncated atom at %d", off)
	}
	size := uint64(binary.BigEndian.Uint32(data[off : off+4]))
	hdr := 8
	toEnd := false
	switch size {
	case 0:
		toEnd = true
		size = uint64(len(data) - off)
	case 1:
		if len(data)-off < 16 {
			return atomLoc{}, fmt.Errorf("tagger: truncated 64-bit atom at %d", off)
		}
		size = binary.BigEndian.Uint64(data[off+8 : off+16])
		hdr = 16
	}
	if size < uint64(hdr) || size > uint64(len(data)-off) || size > math.MaxInt {
		return atomLoc{}, fmt.Errorf("tagger: bad atom size at %d", off)
	}
	end := off + int(size)
	return atomLoc{off: off, end: end, hdr: hdr, toEnd: toEnd, typ: string(data[off+4 : off+8])}, nil
}

func appendAtom(body []byte, typ string, payload []byte) ([]byte, error) {
	var err error
	if body, err = explicitTerminal(body); err != nil {
		return nil, err
	}
	atom, err := buildAtom(typ, payload)
	if err != nil {
		return nil, err
	}
	return append(append([]byte{}, body...), atom...), nil
}

// explicitTerminal rewrites a size-to-end trailing atom with an explicit size
// so an appended sibling lands outside of it.
func explicitTerminal(body []byte) ([]byte, error) {
	for off := 0; off < len(body); {
		a, err := parseAtom(body, off)
		if err != nil {
			return nil, err
		}
		if a.toEnd {
			fixed, err := buildAtom(a.typ, body[a.off+a.hdr:a.end])
			if err != nil {
				return nil, err
			}
			return append(append([]byte{}, body[:a.off]...), fixed...), nil
		}
		off = a.end
	}
	return body, nil
}

func replaceAtom(body []byte, old atomLoc, typ string, payload []byte) ([]byte, bool, error) {
	atom, err := buildAtom(typ, payload)
	if err != nil {
		return nil, false, err
	}
	out := make([]byte, 0, len(body)-old.end+old.off+len(atom))
	out = append(out, body[:old.off]...)
	out = append(out, atom...)
	return append(out, body[old.end:]...), true, nil
}

func buildAtom(typ string, payload []byte) ([]byte, error) {
	if len(typ) != 4 {
		return nil, fmt.Errorf("tagger: bad atom type %q", typ)
	}
	if len(payload) > math.MaxUint32-8 {
		return nil, fmt.Errorf("tagger: atom %s too large", typ)
	}
	atom := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(atom[:4], uint32(len(atom)))
	copy(atom[4:8], typ)
	copy(atom[8:], payload)
	return atom, nil
}

func rewriteMoov(src *os.File, info os.FileInfo, atoms []mp4.TopLevelAtom, moovIdx int, newMoov []byte, closeSrc func() error) error {
	tmp, err := os.CreateTemp(filepath.Dir(src.Name()), "."+filepath.Base(src.Name())+".meta-*")
	if err != nil {
		return fmt.Errorf("tagger: temp for moov rewrite: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		cleanup()
		return fmt.Errorf("tagger: chmod temp: %w", err)
	}
	for i, a := range atoms {
		if i == moovIdx {
			if _, err := tmp.Write(newMoov); err != nil {
				cleanup()
				return fmt.Errorf("tagger: write moov: %w", err)
			}
			continue
		}
		if _, err := src.Seek(a.Offset, io.SeekStart); err != nil {
			cleanup()
			return fmt.Errorf("tagger: seek atom: %w", err)
		}
		if _, err := io.CopyN(tmp, src, a.Size); err != nil {
			cleanup()
			return fmt.Errorf("tagger: copy atom: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("tagger: close temp: %w", err)
	}
	if err := closeSrc(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("tagger: close m4a: %w", err)
	}
	if err := os.Rename(tmpPath, src.Name()); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("tagger: replace m4a: %w", err)
	}
	return nil
}
