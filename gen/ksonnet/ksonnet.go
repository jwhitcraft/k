package ksonnet

import (
	"bytes"
	"regexp"

	"github.com/pkg/errors"

	"github.com/kube-jsonnet/k/gen/kubespec"
	"github.com/kube-jsonnet/k/gen/printer"
)

// Lib is a ksonnet lib.
type Lib struct {
	K8s        map[string][]byte
	Extensions []byte
	Version    string
}

type GenOpts struct {
	// OpenAPI (swagger.json) spec of the API
	OpenAPI string

	// Optional regexp to only generate specific apiGroups
	Target *regexp.Regexp

	// Name of the generated library (embedded into Jsonnet)
	Name string

	// Version of the generated library (embedded into Jsonnet)
	Version string
}

// GenerateLib generates ksonnet lib.
func GenerateLib(opts GenOpts) (*Lib, error) {
	apiSpec, checksum, err := kubespec.Import(opts.OpenAPI)
	if err != nil {
		return nil, errors.Wrap(err, "import Kubernetes spec")
	}

	c, err := NewCatalog(apiSpec,
		CatalogOptChecksum(checksum),
		CatalogOptVersion(opts.Version),
	)

	if err != nil {
		return nil, errors.Wrap(err, "create ksonnet catalog")
	}

	k8s, err := createK8s(c)
	if err != nil {
		return nil, errors.Wrap(err, "create k8s.libsonnet")
	}

	k, err := createK(c)
	if err != nil {
		return nil, errors.Wrap(err, "create k.libsonnet")
	}

	lib := &Lib{
		K8s:        k8s,
		Extensions: k,
		Version:    c.apiVersion.String(),
	}

	return lib, nil
}

func createK8s(c *Catalog) (map[string][]byte, error) {
	doc, err := NewDocument(c)
	if err != nil {
		return nil, errors.Wrapf(err, "create document")
	}

	nodes, err := doc.Nodes()
	if err != nil {
		return nil, errors.Wrapf(err, "build document node")
	}

	out := make(map[string][]byte)
	for name, n := range nodes {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, n.Node()); err != nil {
			return nil, errors.Wrap(err, "print AST")
		}

		out[name] = buf.Bytes()
	}

	return out, nil
}

func createK(c *Catalog) ([]byte, error) {
	e := NewExtension(c)

	node, err := e.Node()
	if err != nil {
		return nil, errors.Wrapf(err, "build extension node")
	}

	var buf bytes.Buffer

	if err := printer.Fprint(&buf, node.Node()); err != nil {
		return nil, errors.Wrap(err, "print AST")
	}

	return buf.Bytes(), nil
}
