package manager

import (
	"github.com/akozlenkov/go-debian/control"
	"github.com/akozlenkov/go-debian/dependency"
)

type Release struct {
	control.Paragraph

	Origin        string
	Suite         string
	Label         string
	Codename      string
	Description   string
	Components    []string
	Architectures []dependency.Arch
	Date          Date
	MD5           []control.MD5FileHash    `control:"MD5Sum" multiline:"true" delim:"\n" strip:"\n\r\t "`
	SHA1          []control.SHA1FileHash   `control:"SHA1" multiline:"true" delim:"\n" strip:"\n\r\t "`
	SHA256        []control.SHA256FileHash `control:"SHA256" multiline:"true" delim:"\n" strip:"\n\r\t "`
}
