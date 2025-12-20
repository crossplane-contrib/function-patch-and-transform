package main

import (
	"testing"

	"github.com/crossplane-contrib/function-patch-and-transform/input/v1beta1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestExtractConnectionDetails(t *testing.T) {
	type args struct {
		cd   resource.Composed
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
