package ksonnet

import (
	"sort"

	"github.com/pkg/errors"

	"github.com/kube-jsonnet/k/gen/log"
	nm "github.com/kube-jsonnet/k/gen/nodemaker"
)

type renderNodeFn func(c *Catalog, a *APIObject) (*nm.Object, error)

// Document represents a ksonnet lib document.
type Document struct {
	catalog *Catalog

	// these are defined to aid testing Document
	typesFn            func() ([]Type, error)
	fieldsFn           func() ([]Field, error)
	renderFn           func(fn renderNodeFn, c *Catalog, o *nm.Object, group Group) error
	renderGroups       func(doc *Document) (map[string]nm.Noder, error)
	renderHiddenGroups func(doc *Document) (nm.Noder, error)
	objectNodeFn       func(c *Catalog, a *APIObject) (*nm.Object, error)
}

// NewDocument creates an instance of Document.
func NewDocument(catalog *Catalog) (*Document, error) {
	if catalog == nil {
		return nil, errors.New("catalog is nil")
	}

	return &Document{
		catalog:            catalog,
		typesFn:            catalog.Types,
		fieldsFn:           catalog.Fields,
		renderFn:           render,
		renderGroups:       renderGroups,
		renderHiddenGroups: renderHiddenGroups,
		objectNodeFn:       apiObjectNode,
	}, nil
}

// Groups returns an alphabetically sorted list of groups.
func (d *Document) Groups() ([]Group, error) {
	resources, err := d.typesFn()
	if err != nil {
		return nil, errors.Wrap(err, "retrieve resources")
	}

	var nodeObjects []Object
	for _, resource := range resources {
		res := resource
		nodeObjects = append(nodeObjects, &res)
	}

	return d.groups(nodeObjects)
}

// HiddenGroups returns an alphabetically sorted list of hidden groups.
func (d *Document) HiddenGroups() ([]Group, error) {
	resources, err := d.fieldsFn()
	if err != nil {
		return nil, errors.Wrap(err, "retrieve types")
	}

	var nodeObjects []Object
	for _, resource := range resources {
		res := resource
		nodeObjects = append(nodeObjects, &res)
	}

	return d.groups(nodeObjects)
}

func (d *Document) groups(resources []Object) ([]Group, error) {
	gMap := make(map[string]*Group)

	for i := range resources {
		res := resources[i]
		name := res.Group()

		g, ok := gMap[name]
		if !ok {
			g = NewGroup(name)
			gMap[name] = g
		}

		g.AddResource(res)
		gMap[name] = g
	}

	var groupNames []string

	for name := range gMap {
		groupNames = append(groupNames, name)
	}

	sort.Strings(groupNames)

	var groups []Group

	for _, name := range groupNames {
		g := gMap[name]
		groups = append(groups, *g)
	}

	return groups, nil
}

// Node converts a document to a node.
func (d *Document) Nodes(meta Metadata) (map[string]nm.Noder, error) {
	main := nm.NewObject()
	metadata := map[string]interface{}{
		d.catalog.title: map[string]interface{}{
			"version":    d.catalog.Version(),
			"checksum":   d.catalog.Checksum(),
			"maintainer": d.catalog.maintainer,
			"generator": map[string]interface{}{
				"vendor":  meta.Vendor,
				"version": meta.Version,
			},
		},
	}

	metadataObj, err := nm.KVFromMap(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "create metadata key")
	}
	main.Set(nm.InheritedKey("__ksonnet"), metadataObj)

	files, err := d.renderGroups(d)
	if err != nil {
		return nil, err
	}

	hidden, err := d.renderHiddenGroups(d)
	if err != nil {
		return nil, err
	}
	files["_hidden"] = hidden

	for k, g := range files {
		g.(*nm.Object).Set0(nm.LocalKey("hidden"), nm.NewImport("_hidden.libsonnet"))
		files[k] = g
	}

	for _, name := range mapKeys(files) {
		main.Set(nm.NewKey(name), nm.NewImport(name+".libsonnet"))
	}
	files["k8s"] = main

	return files, nil
}

func mapKeys(m map[string]nm.Noder) (keys []string) {
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return
}

func render(fn renderNodeFn, catalog *Catalog, o *nm.Object, group Group) error {
	log.Debugln(group.Name())
	for _, version := range group.Versions() {
		versionNode := version.Node()

		log.Debugln(" ", version.Name())
		for _, apiObject := range version.APIObjects() {

			log.Debugln("   ", apiObject.Kind())
			objectNode, err := fn(catalog, &apiObject)
			if err != nil {
				return errors.Wrapf(err, "create node %s", apiObject.Kind())
			}

			versionNode.Set(
				nm.NewKey(apiObject.Kind(), nm.KeyOptComment(apiObject.Description())),
				objectNode)
		}

		o.Set(nm.NewKey(version.Name()), versionNode)
	}

	return nil
}

func renderGroups(d *Document) (map[string]nm.Noder, error) {
	out := make(map[string]nm.Noder)

	groups, err := d.Groups()
	if err != nil {
		return nil, errors.Wrap(err, "retrieve groups")
	}

	for _, g := range groups {
		o := nm.NewObject()

		if err = d.renderFn(d.objectNodeFn, d.catalog, o, g); err != nil {
			return nil, errors.Wrap(err, "render groups")
		}

		out[g.Name()] = o
	}

	return out, nil
}

func renderHiddenGroups(d *Document) (nm.Noder, error) {
	groups, err := d.HiddenGroups()
	if err != nil {
		return nil, errors.Wrap(err, "retrieve hidden groups")
	}

	con := nm.NewObject()
	for _, g := range groups {
		o := nm.NewObject()

		if err = d.renderFn(d.objectNodeFn, d.catalog, o, g); err != nil {
			return nil, errors.Wrap(err, "render hidden groups")
		}

		con.Set(nm.NewKey(g.Name()), o)
	}

	return con, nil
}
