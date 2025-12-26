package main

import (
	"testing"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
)

func TestExtractConnectionDetails(t *testing.T) {
	type args struct {
		cd   xpresource.Composed
		data managed.ConnectionDetails
		cfg  []v1beta1.ConnectionDetail
	}
	type want struct {
		conn managed.ConnectionDetails
		err  error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"MissingNameError": {
			reason: "We should return an error if a connection detail is missing a name.",
			args: args{
				cfg: []v1beta1.ConnectionDetail{
					{
						// A nameless connection detail.
						Type: v1beta1.ConnectionDetailTypeFromValue,
					},
				},
			},
			want: want{
				err: errors.Wrap(errors.New("name: Required value: name is required"), "invalid"),
			},
		},
		"FetchConfigSuccess": {
			reason: "Should extract only the selected set of secret keys",
			args: args{
				cd: &fake.Composed{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Generation: 4,
					},
					ConnectionSecretWriterTo: fake.ConnectionSecretWriterTo{
						Ref: &xpv1.SecretReference{
							Name:      "cool-secret",
							Namespace: "cool-namespace",
						},
					},
				},
				data: managed.ConnectionDetails{
					"foo": []byte("a"),
					"bar": []byte("b"),
				},
				cfg: []v1beta1.ConnectionDetail{
					{
						Type:                    v1beta1.ConnectionDetailTypeFromConnectionSecretKey,
						Name:                    "bar",
						FromConnectionSecretKey: ptr.To[string]("bar"),
					},
					{
						Type:                    v1beta1.ConnectionDetailTypeFromConnectionSecretKey,
						Name:                    "none",
						FromConnectionSecretKey: ptr.To[string]("none"),
					},
					{
						Type:                    v1beta1.ConnectionDetailTypeFromConnectionSecretKey,
						Name:                    "convfoo",
						FromConnectionSecretKey: ptr.To[string]("foo"),
					},
					{
						Type:  v1beta1.ConnectionDetailTypeFromValue,
						Name:  "fixed",
						Value: ptr.To[string]("value"),
					},
					// Note that the FromFieldPath values don't include their
					// initial path segment due to being anonymous embedded
					// fields in the fake.Composed struct.
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "name",
						FromFieldPath: ptr.To[string]("name"),
					},
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "generation",
						FromFieldPath: ptr.To[string]("generation"),
					},
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "secretName",
						FromFieldPath: ptr.To[string]("Ref.name"),
					},
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "secretNamespace",
						FromFieldPath: ptr.To[string]("Ref.namespace"),
					},
				},
			},
			want: want{
				conn: managed.ConnectionDetails{
					"convfoo":         []byte("a"),
					"bar":             []byte("b"),
					"fixed":           []byte("value"),
					"name":            []byte("test"),
					"generation":      []byte("4"),
					"secretName":      []byte("cool-secret"),
					"secretNamespace": []byte("cool-namespace"),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			conn, err := ExtractConnectionDetails(tc.args.cd, tc.args.data, tc.args.cfg...)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nExtractConnectionDetails(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.conn, conn, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("\n%s\nExtractConnectionDetails(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestGetConnectionSecretRef(t *testing.T) {
	type args struct {
		xr    *resource.Composite
		input *v1beta1.WriteConnectionSecretToRef
	}
	type want struct {
		ref xpv1.SecretReference
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"InputProvided": {
			reason: "Should use the provided name and namespace from function input when provided",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"uid":"test-uid-123"}
						}`), xr)
						return xr
					}(),
				},
				input: &v1beta1.WriteConnectionSecretToRef{
					Name:      "my-custom-secret",
					Namespace: "custom-namespace",
				},
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "my-custom-secret",
					Namespace: "custom-namespace",
				},
			},
		},
		"XRHasWriteConnectionSecretToRef": {
			reason: "Should use the XR's writeConnectionSecretToRef when no input is provided",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"uid":"test-uid-456"},
							"spec":{"writeConnectionSecretToRef":{"name":"xr-secret","namespace":"xr-namespace"}}
						}`), xr)
						return xr
					}(),
				},
				input: nil,
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "xr-secret",
					Namespace: "xr-namespace",
				},
			},
		},
		"GenerateDefault": {
			reason: "Should generate name from XR name when neither input nor XR ref is provided",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"name":"my-xr","namespace":"xr-namespace","uid":"test-uid-789"}
						}`), xr)
						return xr
					}(),
				},
				input: nil,
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "my-xr-connection",
					Namespace: "xr-namespace",
				},
			},
		},
		"PatchFromCompositeFieldPath": {
			reason: "Should apply patches to transform the secret name using XR metadata",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"name":"my-database","namespace":"production","uid":"test-uid-456"},
							"spec":{"parameters":{"env":"prod"}}
						}`), xr)
						return xr
					}(),
				},
				input: &v1beta1.WriteConnectionSecretToRef{
					Patches: []v1beta1.ConnectionSecretPatch{
						{
							Type: v1beta1.PatchTypeFromCompositeFieldPath,
							Patch: v1beta1.Patch{
								FromFieldPath: ptr.To("metadata.uid"),
								ToFieldPath:   ptr.To("name"),
								Transforms: []v1beta1.Transform{
									{
										Type: v1beta1.TransformTypeString,
										String: &v1beta1.StringTransform{
											Type:   v1beta1.StringTransformTypeFormat,
											Format: ptr.To("%s-cool-creds"),
										},
									},
								},
							},
						},
						{
							Type: v1beta1.PatchTypeFromCompositeFieldPath,
							Patch: v1beta1.Patch{
								FromFieldPath: ptr.To("spec.parameters.env"),
								ToFieldPath:   ptr.To("namespace"),
							},
						},
					},
				},
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "test-uid-456-cool-creds",
					Namespace: "prod",
				},
			},
		},
		"CombineFromComposite": {
			reason: "Should apply combine patches to construct secret name from multiple fields",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"name":"db-instance","namespace":"default"},
							"spec":{"parameters":{"appName":"myapp","env":"staging"}}
						}`), xr)
						return xr
					}(),
				},
				input: &v1beta1.WriteConnectionSecretToRef{
					Patches: []v1beta1.ConnectionSecretPatch{
						{
							Type: v1beta1.PatchTypeCombineFromComposite,
							Patch: v1beta1.Patch{
								Combine: &v1beta1.Combine{
									Variables: []v1beta1.CombineVariable{
										{FromFieldPath: "spec.parameters.appName"},
										{FromFieldPath: "spec.parameters.env"},
									},
									Strategy: v1beta1.CombineStrategyString,
									String: &v1beta1.StringCombine{
										Format: "%s-%s-cool-creds",
									},
								},
								ToFieldPath: ptr.To("name"),
							},
						},
					},
				},
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "myapp-staging-cool-creds",
					Namespace: "default", // we didn't patch this, but it picks it up from the XR's namespace
				},
			},
		},
		"PatchesWithStaticBase": {
			reason: "Should apply patches on top of static input values, basically overwriting them. if no patch exists for a field then the static base should be used.",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"name":"my-xr","namespace":"default"},
							"spec":{"targetNamespace":"custom-ns"}
						}`), xr)
						return xr
					}(),
				},
				input: &v1beta1.WriteConnectionSecretToRef{
					Name:      "base-secret-name",
					Namespace: "base-secret-namespace",
					Patches: []v1beta1.ConnectionSecretPatch{
						{
							Type: v1beta1.PatchTypeFromCompositeFieldPath,
							Patch: v1beta1.Patch{
								FromFieldPath: ptr.To("spec.targetNamespace"),
								ToFieldPath:   ptr.To("namespace"),
							},
						},
					},
				},
			},
			want: want{
				ref: xpv1.SecretReference{
					Name:      "base-secret-name", // only namespace was patched
					Namespace: "custom-ns",
				},
			},
		},
		"UnsupportedPatchType": {
			reason: "Should return an error when an unsupported patch is provided.",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"name":"my-xr","namespace":"default"},
						}`), xr)
						return xr
					}(),
				},
				input: &v1beta1.WriteConnectionSecretToRef{
					Patches: []v1beta1.ConnectionSecretPatch{
						{
							Type: v1beta1.PatchTypeCombineToEnvironment,
						},
					},
				},
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := getConnectionSecretRef(tc.args.xr, tc.args.input)
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetConnectionSecretRef(...): -want, +got:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.ref, got); diff != "" {
				t.Errorf("%s\ngetConnectionSecretRef(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestSupportsConnectionDetails(t *testing.T) {
	type args struct {
		xr *resource.Composite
	}
	type want struct {
		supportsConnectionDetails bool
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"V1LegacyXRWithoutCrossplaneField": {
			reason: "A legacy v1 XR without spec.crossplane field should support connection details",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"uid":"test-uid"},
							"spec":{"writeConnectionSecretToRef":{"name":"my-secret"}}
						}`), xr)
						return xr
					}(),
				},
			},
			want: want{
				supportsConnectionDetails: true,
			},
		},
		"V2ModernXRWithCrossplaneField": {
			reason: "A v2 XR with spec.crossplane field should not support connection details",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"uid":"test-uid"},
							"spec":{"crossplane":{"compositionRef":{"name":"my-comp"}}}
						}`), xr)
						return xr
					}(),
				},
			},
			want: want{
				supportsConnectionDetails: false,
			},
		},
		"V2XRWithWriteConnectionSecretToRefInSchema": {
			reason: "A v2 XR that also has writeConnectionSecretToRef (manually added by XR author for compatibility) should still not support connection details based on presence of spec.crossplane",
			args: args{
				xr: &resource.Composite{
					Resource: func() *composite.Unstructured {
						xr := composite.New()
						_ = json.Unmarshal([]byte(`{
							"apiVersion":"example.org/v1",
							"kind":"XR",
							"metadata":{"uid":"test-uid"},
							"spec":{
								"crossplane":{"compositionRef":{"name":"my-comp"}},
								"writeConnectionSecretToRef":{"name":"my-secret"}
							}
						}`), xr)
						return xr
					}(),
				},
			},
			want: want{
				supportsConnectionDetails: false,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := supportsConnectionDetails(tc.args.xr)
			if got != tc.want.supportsConnectionDetails {
				t.Errorf("%s\nsupportsConnectionDetails(...): want %v, got %v", tc.reason, tc.want.supportsConnectionDetails, got)
			}
		})
	}
}
