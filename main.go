// chessbase_extractor pulls the ChessBase database files (.2cbh & co.) out of
// a Windows "SetupMega…exe" installer without running it.
//
// Layout of the installer (verified on SetupMega2025_2cbh_v2.exe):
//
//	NSIS installer (stored, uncompressed)
//	└── Mega20252cbh.exe      7-Zip self-extracting stub
//	    └── 7z archive        the actual database files
//
// Because the NSIS layer stores its payload uncompressed, the 7z archive sits
// verbatim inside the outer .exe. The program therefore scans for a 7z
// signature header, validates it (CRC + bounds) to rule out false positives,
// and unpacks that archive straight from the installer.
//
// Usage:
//
//	go run . [-o outdir] [-list] [-p password] [installer.exe]
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bodgit/sevenzip"
)

var sevenZipMagic = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}

const startHeaderSize = 32 // magic(6) version(2) crc(4) nextOff(8) nextSize(8) nextCRC(4)

func main() {
	outDir := flag.String("o", ".", "output directory")
	listOnly := flag.Bool("list", false, "only list the embedded files, do not extract")
	password := flag.String("p", "", "archive password (only if the archive is encrypted)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] [installer.exe]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(flag.Arg(0), *outDir, *password, *listOnly); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(exePath, outDir, password string, listOnly bool) error {
	if exePath == "" {
		var err error
		if exePath, err = findInstaller(); err != nil {
			return err
		}
	}

	f, err := os.Open(exePath)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	fmt.Printf("Scanning %s for an embedded 7z archive...\n", filepath.Base(exePath))
	off, size, err := findArchive(f, st.Size())
	if err != nil {
		return err
	}
	fmt.Printf("Found 7z archive at offset %d (%s)\n", off, human(size))

	section := io.NewSectionReader(f, off, size)
	var zr *sevenzip.Reader
	if password != "" {
		zr, err = sevenzip.NewReaderWithPassword(section, size, password)
	} else {
		zr, err = sevenzip.NewReader(section, size)
	}
	if err != nil {
		return fmt.Errorf("opening embedded 7z archive: %w", err)
	}

	var total uint64
	for _, zf := range zr.File {
		total += zf.UncompressedSize
	}

	if listOnly {
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			fmt.Printf("%12d  %s\n", zf.UncompressedSize, zf.Name)
		}
		fmt.Printf("%d entries, %s uncompressed\n", len(zr.File), human(int64(total)))
		return nil
	}

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	fmt.Printf("Extracting %d entries (%s) to %s\n", len(zr.File), human(int64(total)), absOut)

	p := &progress{total: total, start: time.Now()}
	// Entries are extracted in archive order: 7z archives are usually solid,
	// and sequential access avoids re-decompressing earlier data.
	for _, zf := range zr.File {
		if err := extract(zf, absOut, p); err != nil {
			return fmt.Errorf("%s: %w", zf.Name, err)
		}
	}
	p.finish()
	return nil
}

// findInstaller picks the installer when none is given on the command line:
// the only .exe in the current directory.
func findInstaller() (string, error) {
	matches, err := filepath.Glob("*.exe")
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", errors.New("no .exe found in the current directory; pass the installer path as argument")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("several .exe files found (%s); pass the one to extract as argument", strings.Join(matches, ", "))
	}
}

// findArchive returns the offset and length of the first valid 7z archive
// embedded in r.
func findArchive(r io.ReaderAt, fileSize int64) (off, size int64, err error) {
	const chunk = 4 << 20
	overlap := len(sevenZipMagic) - 1
	buf := make([]byte, chunk+overlap)

	for pos := int64(0); pos < fileSize; pos += chunk {
		n, rerr := r.ReadAt(buf, pos)
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return 0, 0, rerr
		}
		data := buf[:n]
		for i := 0; ; {
			j := bytes.Index(data[i:], sevenZipMagic)
			if j < 0 {
				break
			}
			cand := pos + int64(i+j)
			if sz, ok := validArchive(r, cand, fileSize); ok {
				return cand, sz, nil
			}
			i += j + 1
		}
	}
	return 0, 0, errors.New("no embedded 7z archive found (unsupported installer layout)")
}

// validArchive checks the 7z signature header at off: its CRC must match and
// the trailing header it points to must lie inside the file.
func validArchive(r io.ReaderAt, off, fileSize int64) (int64, bool) {
	var h [startHeaderSize]byte
	if _, err := r.ReadAt(h[:], off); err != nil {
		return 0, false
	}
	if crc32.ChecksumIEEE(h[12:]) != binary.LittleEndian.Uint32(h[8:12]) {
		return 0, false
	}
	nextOff := binary.LittleEndian.Uint64(h[12:20])
	nextSize := binary.LittleEndian.Uint64(h[20:28])
	remaining := uint64(fileSize - off - startHeaderSize)
	if nextOff > remaining || nextSize > remaining-nextOff {
		return 0, false
	}
	return int64(startHeaderSize + nextOff + nextSize), true
}

func extract(zf *sevenzip.File, outDir string, p *progress) error {
	// Archive was built on Windows: normalise separators and refuse any
	// path that would escape the output directory.
	name := filepath.FromSlash(strings.ReplaceAll(zf.Name, `\`, "/"))
	if !filepath.IsLocal(name) {
		return errors.New("unsafe path in archive, refusing to extract")
	}
	dest := filepath.Join(outDir, name)

	if zf.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	p.name = name
	if _, err := io.Copy(io.MultiWriter(out, p), rc); err != nil {
		out.Close()
		return err
	}
	// Closing the reader verifies the entry's CRC.
	if err := rc.Close(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if !zf.Modified.IsZero() {
		_ = os.Chtimes(dest, zf.Modified, zf.Modified)
	}
	return nil
}

// progress is an io.Writer that prints a single-line progress indicator.
type progress struct {
	total, done uint64
	name        string
	start, last time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += uint64(len(b))
	if time.Since(p.last) > 200*time.Millisecond {
		p.last = time.Now()
		p.print()
	}
	return len(b), nil
}

func (p *progress) print() {
	pct := 100.0
	if p.total > 0 {
		pct = float64(p.done) / float64(p.total) * 100
	}
	fmt.Printf("\r\033[K%5.1f%%  %s / %s  %s", pct, human(int64(p.done)), human(int64(p.total)), p.name)
}

func (p *progress) finish() {
	fmt.Printf("\r\033[KDone: %s extracted in %s\n", human(int64(p.done)), time.Since(p.start).Round(time.Second))
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
