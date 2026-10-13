package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Method is how the running copy was installed, and so how to update it.
type Method int

const (
	Archive   Method = iota // a release archive, extracted by hand
	Homebrew                // brew install krank56/tap/jenklod-batman
	GoInstall               // go install …@version
)

func (m Method) String() string {
	switch m {
	case Homebrew:
		return "Homebrew"
	case GoInstall:
		return "go install"
	}
	return "the release archive"
}

// Detect tells the method from the binary's resolved path and whether its
// version came from Go's build info.
func Detect(exe string, fromBuildInfo bool) Method {
	switch {
	case strings.Contains(exe, "/Cellar/jenklod-batman/"):
		return Homebrew
	case fromBuildInfo:
		return GoInstall
	}
	return Archive
}

// ErrTapBehind means brew upgraded to what its tap has, which is not the
// latest release yet.
var ErrTapBehind = errors.New("the Homebrew tap doesn't have it yet: it picks new releases up within the hour, try again shortly")

const formula = "krank56/tap/jenklod-batman"

// Installer updates the running copy.
type Installer struct {
	Source       *Source
	Exe          string // the running binary, symlinks resolved
	GOOS, GOARCH string
	// Run runs a command on the terminal; Output runs one for its output.
	Run    func(name string, args ...string) error
	Output func(name string, args ...string) ([]byte, error)
}

// NewInstaller updates exe for this platform.
func NewInstaller(exe string) *Installer {
	return &Installer{
		Source: NewSource(),
		Exe:    exe,
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		Run: func(name string, args ...string) error {
			cmd := exec.Command(name, args...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return cmd.Run()
		},
		Output: func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).Output() },
	}
}

// Install installs version to with method m.
func (in *Installer) Install(ctx context.Context, m Method, to Version) error {
	switch m {
	case Homebrew:
		if err := in.Run("brew", "upgrade", formula); err != nil {
			return fmt.Errorf("brew upgrade: %w", err)
		}
		out, err := in.Output("brew", "list", "--versions", "jenklod-batman")
		if err != nil {
			return fmt.Errorf("brew list: %w", err)
		}
		// "jenklod-batman 0.3.0 0.4.0": the name, then each installed version.
		for _, f := range strings.Fields(string(out)) {
			if v, ok := Parse(f); ok && v == to {
				return nil
			}
		}
		return ErrTapBehind
	case GoInstall:
		if err := in.Run("go", "install", Module+"@"+to.String()); err != nil {
			return fmt.Errorf("go install: %w", err)
		}
		return nil
	}
	return in.replace(ctx, to)
}

// replace downloads the release archive for this platform, checks it
// against the release's checksums.txt, and swaps the binary in place.
func (in *Installer) replace(ctx context.Context, to Version) error {
	name := fmt.Sprintf("jenklod-batman_%s_%s_%s", to, in.GOOS, in.GOARCH)
	base := in.Source.Download + "/" + to.String() + "/"
	sums, err := in.Source.get(ctx, base+"checksums.txt")
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name+".tar.gz" {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("%s has no build for %s/%s", to, in.GOOS, in.GOARCH)
	}
	tgz, err := in.Source.get(ctx, base+name+".tar.gz")
	if err != nil {
		return err
	}
	if got := sha256.Sum256(tgz); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s.tar.gz doesn't match its checksum: nothing was changed", name)
	}
	bin, err := extract(tgz, name+"/jenklod-batman")
	if err != nil {
		return err
	}

	// A new file in the same directory, renamed over the old one: the
	// running binary is never half written.
	dir := filepath.Dir(in.Exe)
	tmp, err := os.CreateTemp(dir, ".jenklod-batman-*")
	if err != nil {
		return fmt.Errorf("can't write to %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), in.Exe)
}

func extract(tgz []byte, path string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("the archive has no %s", path)
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimPrefix(h.Name, "./") == path {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}
