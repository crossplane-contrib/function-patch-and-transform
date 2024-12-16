package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	"github.com/crossplane-contrib/function-patch-and-transform/pt"
)

// ConnectionDetailsExtractor extracts the connection details of a resource.
type ConnectionDetailsExtractor interface {
	// ExtractConnection of the supplied resource.
	ExtractConnection(cd resource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error)
}

// A ConnectionDetailsExtractorFn is a function that satisfies
// ConnectionDetailsExtractor.
type ConnectionDetailsExtractorFn func(cd resource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error)

// ExtractConnection of the supplied resource.
func (fn ConnectionDetailsExtractorFn) ExtractConnection(cd resource.Composed, conn managed.ConnectionDetails, cfg ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error) {
	return fn(cd, conn, cfg...)
}

// ExtractConnectionDetails extracts XR connection details from the supplied
// composed resource. If no ExtractConfigs are supplied no connection details
// will be returned.
func ExtractConnectionDetails(cd resource.Composed, data managed.ConnectionDetails, cfgs ...v1beta1.ConnectionDetail) (managed.ConnectionDetails, error) {
	out := map[string][]byte{}
	for _, cfg := range cfgs {
		if err := pt.ValidateConnectionDetail(cfg); err != nil {
			return nil, errors.Wrap(err, "invalid")
		}
		switch cfg.Type {
		case v1beta1.ConnectionDetailTypeFromValue:
			out[cfg.Name] = []byte(*cfg.Value)
		case v1beta1.ConnectionDetailTypeFromConnectionSecretKey:
			if data[*cfg.FromConnectionSecretKey] == nil {
				// We don't consider this an error because it's possible the
				// key will still be written at some point in the future.
				continue
			}
			out[cfg.Name] = data[*cfg.FromConnectionSecretKey]
		case v1beta1.ConnectionDetailTypeFromFieldPath:
			// Note we're checking that the error _is_ nil. If we hit an error
			// we silently avoid including this connection secret. It's possible
			// the path will start existing with a valid value in future.
			if b, err := fromFieldPath(cd, *cfg.FromFieldPath); err == nil {
				out[cfg.Name] = b
			}
		}
	}
	return out, nil
}

// fromFieldPath tries to read the value from the supplied field path first as a
// plain string. If this fails, it falls back to reading it as JSON.
func fromFieldPath(from runtime.Object, path string) ([]byte, error) {
	fromMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return nil, err
	}

	str, err := fieldpath.Pave(fromMap).GetString(path)
	if err == nil {
		return []byte(str), nil
	}

	in, err := fieldpath.Pave(fromMap).GetValue(path)
	if err != nil {
		return nil, err
	}

	return json.Marshal(in)
}
