package manager

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/akozlenkov/faptly/config"
	"github.com/akozlenkov/faptly/pgp"
	"github.com/akozlenkov/faptly/storage"
	"github.com/akozlenkov/go-debian/control"
	"github.com/akozlenkov/go-debian/dependency"
	"github.com/akozlenkov/go-debian/hashio"
	"github.com/blakesmith/ar"
	"github.com/klauspost/compress/zstd"
	"github.com/xi2/xz"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	OS           = "linux"
	ABI          = "gnu"
	PoolDir      = "pool"
	DistsDir     = "dists"
	ReleaseFile  = "InRelease"
	PackagesFile = "Packages"
)

type Manager struct {
	mu      sync.Mutex
	index   map[string][]control.BinaryIndex
	config  *config.Config
	storage storage.Storage
}

func New(c *config.Config) (*Manager, error) {
	s, err := storage.New(c.S3Endpoint, c.S3Bucket, c.S3AccessKey, c.S3SecretKey)
	if err != nil {
		return nil, err
	}

	return &Manager{
		index:   make(map[string][]control.BinaryIndex),
		config:  c,
		storage: s,
	}, nil
}

func (m *Manager) ListRepos() error {
	sb := new(strings.Builder)
	if err := m.storage.Walk(path.Join(DistsDir), func(path string, err error) error {
		if err != nil {
			return err
		}

		if match, err := regexp.MatchString(`dists/(\w+)/InRelease$`, path); err == nil && match {
			release := new(Release)

			file, err := m.storage.ReadFile(path)
			if err != nil {
				return err
			}

			if err := control.Unmarshal(release, bytes.NewReader(file)); err != nil {
				return err
			}

			architectures := make([]string, 0, len(release.Architectures))
			for _, arch := range release.Architectures {
				architectures = append(architectures, arch.CPU)
			}
			sb.WriteString(fmt.Sprintf(
				" * %s [%s] (%s): %s\n",
				release.Suite,
				strings.Join(release.Components, ", "),
				strings.Join(architectures, "|"),
				release.Description),
			)
		}

		return nil
	}); err != nil {
		return err
	}

	if sb.Len() != 0 {
		fmt.Printf("List of repositories:\n%s\nTo get more information about local repository, run `faptly repo show <codename>`.\n", sb.String())
	} else {
		fmt.Printf("No repositories found, create one with `faptly repo create ...`.\n")
	}

	return nil
}

func (m *Manager) ShowRepo(suite string) error {
	if m.repoExists(suite) {
		release := new(Release)

		file, err := m.storage.ReadFile(path.Join(DistsDir, suite, ReleaseFile))
		if err != nil {
			return err
		}

		if err := control.Unmarshal(release, bytes.NewReader(file)); err != nil {
			return err
		}

		return control.Marshal(os.Stdout, release)
	}

	return fmt.Errorf("repository %s not found", suite)
}

func (m *Manager) CreateRepo(origin, suite, label, codename, description string, components []string, architectures []string) error {
	if !m.repoExists(suite) {
		for _, component := range components {
			for _, arch := range architectures {
				if err := m.storage.WriteFile(path.Join(DistsDir, suite, component, "binary-"+arch, PackagesFile), []byte{}); err != nil {
					return err
				}
			}
		}

		arch := make([]dependency.Arch, len(architectures))
		for i, a := range architectures {
			arch[i] = dependency.Arch{
				OS:  OS,
				ABI: ABI,
				CPU: a,
			}
		}

		return m.rebuildRelease(&Release{
			Origin:        origin,
			Label:         label,
			Suite:         suite,
			Codename:      codename,
			Components:    components,
			Description:   description,
			Architectures: arch,
		})
	}
	return fmt.Errorf("repository %s already exists", suite)
}

func (m *Manager) DeleteRepo(suite string) error {
	if m.repoExists(suite) {
		for _, dir := range []string{PoolDir, DistsDir} {
			if err := m.storage.RemoveAll(path.Join(dir, suite)); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("repository %s not found", suite)
}

func (m *Manager) ListPkgs(suite, component, architecture string) error {
	sb := new(strings.Builder)
	if m.repoExists(suite) {
		release, err := m.getRelease(suite)
		if err != nil {
			return err
		}

		if !slices.Contains(release.Components, component) {
			return fmt.Errorf("unsuppored component")
		}

		if !slices.Contains(release.Architectures, dependency.Arch{ABI: "gnu", OS: "linux", CPU: architecture}) {
			return fmt.Errorf("unsuppored architecture")
		}

		indexes, err := m.getBinaryIndexes(path.Join(DistsDir, suite, component, "binary-"+architecture, PackagesFile))
		if err != nil {
			return err
		}

		for _, index := range indexes {
			sb.WriteString(fmt.Sprintf(" - %s\n", filepath.Base(index.Filename)))
		}

		if sb.Len() != 0 {
			fmt.Printf("List of packages:\n%s\nTo get more information about package, run `faptly pkg show <package>`.\n", sb.String())
		} else {
			fmt.Printf("No packages found, upload one with `faptly pkg upload ...`.\n")
		}

		return nil
	}

	return fmt.Errorf("repository %s not found", suite)
}

func (m *Manager) ShowPkg(suite, component, architecture, pkg string) error {
	if m.repoExists(suite) {
		release, err := m.getRelease(suite)
		if err != nil {
			return err
		}

		if !slices.Contains(release.Components, component) {
			return fmt.Errorf("unsuppored component")
		}

		if !slices.Contains(release.Architectures, dependency.Arch{ABI: "gnu", OS: "linux", CPU: architecture}) {
			return fmt.Errorf("unsuppored architecture")
		}

		indexes, err := m.getBinaryIndexes(path.Join(DistsDir, suite, component, "binary-"+architecture, PackagesFile))
		if err != nil {
			return err
		}

		for _, index := range indexes {
			if pkg == filepath.Base(index.Filename) {
				return control.Marshal(os.Stdout, index)
			}
		}
		return nil
	}

	return fmt.Errorf("repository %s not found", suite)
}

func (m *Manager) UploadPkgs(suite string, component string, pkgs []string) error {
	if m.repoExists(suite) {
		r, err := m.getRelease(suite)
		if err != nil {
			return err
		}

		for _, arch := range r.Architectures {
			i, err := m.getBinaryIndexes(path.Join(DistsDir, suite, component, "binary-"+arch.CPU, PackagesFile))
			if err != nil {
				return err
			}
			m.index[arch.CPU] = i
		}

		sem := semaphore.NewWeighted(int64(runtime.NumCPU()))
		group, ctx := errgroup.WithContext(context.Background())

		for _, pkg := range pkgs {
			func(pkg string) {
				sem.Acquire(ctx, 1)

				group.Go(func() error {
					defer func() {
						sem.Release(1)
						fmt.Printf("Upload package %s\n", pkg)
					}()

					idx := new(control.BinaryIndex)

					data, err := os.ReadFile(pkg)
					if err != nil {
						return err
					}

					reader := bytes.NewReader(data)

					rawIndex, err := readControlFile(reader)
					if err != nil {
						return err
					}

					if err := control.Unmarshal(idx, bufio.NewReader(bytes.NewReader(rawIndex))); err != nil {
						return err
					}
					if idx.Architecture.CPU != "all" && !slices.Contains(r.Architectures, idx.Architecture) {
						return fmt.Errorf("package %s has unsuported architecture %s", path.Base(pkg), idx.Architecture.CPU)
					}

					idx.Size = int(reader.Size())

					_, hashers, err := hashio.NewHasherReaders([]string{"md5", "sha1", "sha256"}, reader)
					for _, h := range hashers {
						switch h.Name() {
						case "md5":
							idx.MD5sum = control.FileHashFromHasher(pkg, *h).Hash
						case "sha1":
							idx.SHA1 = control.FileHashFromHasher(pkg, *h).Hash
						case "sha256":
							idx.SHA256 = control.FileHashFromHasher(pkg, *h).Hash
						}
					}

					if source, ok := idx.Values["Source"]; ok {
						idx.Filename = path.Join(PoolDir, suite, component, strings.Split(source, "")[0], source, path.Base(pkg))
					} else {
						idx.Filename = path.Join(PoolDir, suite, component, strings.Split(idx.Package, "")[0], idx.Package, path.Base(pkg))
					}

					m.mu.Lock()

					if idx.Architecture.CPU == "all" {
						for _, arch := range r.Architectures {
							for i, index := range m.index[arch.CPU] {
								if path.Base(pkg) == path.Base(index.Filename) {
									m.index[arch.CPU] = append(m.index[arch.CPU][:i], m.index[arch.CPU][i+1:]...)
								}
							}
							m.index[arch.CPU] = append(m.index[arch.CPU], *idx)
						}
					} else {
						for i, index := range m.index[idx.Architecture.CPU] {
							if path.Base(pkg) == path.Base(index.Filename) {
								m.index[idx.Architecture.CPU] = append(m.index[idx.Architecture.CPU][:i], m.index[idx.Architecture.CPU][i+1:]...)
							}
						}
						m.index[idx.Architecture.CPU] = append(m.index[idx.Architecture.CPU], *idx)
					}

					m.mu.Unlock()

					if err := m.storage.WriteFile(path.Join(idx.Filename), data); err != nil {
						return err
					}
					return nil
				})
			}(pkg)
		}

		if err := group.Wait(); err != nil {
			return err
		}

		return m.writeBinaryIndexes(r, component, m.index)
	}

	return fmt.Errorf("repository %s doesn't exist", suite)
}

func (m *Manager) repoExists(suite string) bool {
	return m.storage.Exists(path.Join(DistsDir, suite, ReleaseFile))
}

func (m *Manager) rebuildRelease(release *Release) error {
	release.MD5 = make([]control.MD5FileHash, 0)
	release.SHA1 = make([]control.SHA1FileHash, 0)
	release.SHA256 = make([]control.SHA256FileHash, 0)

	if err := m.storage.Walk(path.Join(DistsDir, release.Suite), func(found string, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(found, PackagesFile) {
			return nil
		}

		file, err := m.storage.ReadFile(found)
		if err != nil {
			return err
		}

		p := strings.TrimPrefix(found, path.Join(DistsDir, release.Suite)+"/")

		hashers := []struct {
			Name string
			Hash hash.Hash
		}{
			{"md5", md5.New()},
			{"sha1", sha1.New()},
			{"sha256", sha256.New()},
		}

		writers := make([]io.Writer, len(hashers))
		for i, h := range hashers {
			writers[i] = h.Hash
		}

		if _, err := io.Copy(io.MultiWriter(writers...), bytes.NewReader(file)); err != nil {
			return err
		}

		size := int64(len(file))

		for _, h := range hashers {
			fh := control.FileHash{
				Algorithm: h.Name,
				Hash:      fmt.Sprintf("%x", h.Hash.Sum(nil)),
				Size:      size,
				ByHash:    fmt.Sprintf("%x", h.Hash.Sum(nil)),
				Filename:  p,
			}

			switch h.Name {
			case "md5":
				release.MD5 = append(release.MD5, control.MD5FileHash{FileHash: fh})
			case "sha1":
				release.SHA1 = append(release.SHA1, control.SHA1FileHash{FileHash: fh})
			case "sha256":
				release.SHA256 = append(release.SHA256, control.SHA256FileHash{FileHash: fh})
			}
		}

		return nil
	}); err != nil {
		return err
	}

	return m.writeRelease(release)
}

func (m *Manager) getRelease(suite string) (*Release, error) {
	release := &Release{}

	data, err := m.storage.ReadFile(path.Join(DistsDir, suite, ReleaseFile))
	if err != nil {
		return nil, err
	}

	if err := control.Unmarshal(release, bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return release, nil
}

func (m *Manager) writeRelease(release *Release) error {
	var buf bytes.Buffer

	release.Date = Date{time.Now().UTC()}

	if err := control.Marshal(&buf, release); err != nil {
		return err
	}

	data, err := pgp.SignData([]byte(m.config.PrivateGPGKey), []byte(m.config.PrivateGPGPasskey), buf.Bytes())
	if err != nil {
		return err
	}

	return m.storage.WriteFile(path.Join(DistsDir, release.Suite, ReleaseFile), data)
}

func (m *Manager) getBinaryIndexes(p string) ([]control.BinaryIndex, error) {
	b, err := m.storage.ReadFile(p)
	if err != nil {
		return nil, err
	}

	binaryIndexes, err := control.ParseBinaryIndex(bufio.NewReader(bytes.NewReader(b)))
	if err != nil {
		return nil, err
	}
	return binaryIndexes, nil
}

func (m *Manager) writeBinaryIndexes(release *Release, component string, indexes map[string][]control.BinaryIndex) error {
	for k, index := range indexes {
		var buf bytes.Buffer

		if err := control.Marshal(&buf, index); err != nil {
			return err
		}

		if err := m.storage.WriteFile(path.Join(DistsDir, release.Suite, component, "binary-"+k, PackagesFile), buf.Bytes()); err != nil {
			return err
		}
	}

	return m.rebuildRelease(release)
}

func readControlFile(reader io.Reader) ([]byte, error) {
	archiveReader := ar.NewReader(reader)

	for {
		header, err := archiveReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(header.Name, "control.tar") {
			var controlReader *tar.Reader
			switch path.Ext(strings.Trim(header.Name, "/")) {
			case ".xz":
				stream, err := xz.NewReader(archiveReader, 0)
				if err != nil {
					panic(err)
				}
				controlReader = tar.NewReader(stream)
			case ".gz":
				stream, err := gzip.NewReader(archiveReader)
				if err != nil {
					panic(err)
				}
				controlReader = tar.NewReader(stream)
			case ".zst":
				stream, err := zstd.NewReader(archiveReader)
				if err != nil {
					panic(err)
				}
				controlReader = tar.NewReader(stream)
			case ".bz2":
				controlReader = tar.NewReader(bzip2.NewReader(archiveReader))
			default:
				return nil, fmt.Errorf("compression type %s not supported", path.Ext(strings.Trim(header.Name, "/")))
			}

			for {
				header, err := controlReader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					panic(err)
				}

				if strings.HasSuffix(header.Name, "control") {
					var buffer bytes.Buffer
					_, err := io.Copy(bufio.NewWriter(&buffer), controlReader)
					if err != nil {
						return nil, err
					}
					return buffer.Bytes(), nil
				}
			}
		}
	}
	return nil, errors.New("couldn't find control file in package")
}
