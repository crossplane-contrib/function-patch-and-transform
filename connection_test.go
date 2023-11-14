package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/stevendborrelli/function-conditional-patch-and-transform/input/v1beta1"
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
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "name",
						FromFieldPath: ptr.To[string]("objectMeta.name"),
					},
					{
						Type:          v1beta1.ConnectionDetailTypeFromFieldPath,
						Name:          "generation",
						FromFieldPath: ptr.To[string]("objectMeta.generation"),
					},
				},
			},
			want: want{
				conn: managed.ConnectionDetails{
					"convfoo":    []byte("a"),
					"bar":        []byte("b"),
					"fixed":      []byte("value"),
					"name":       []byte("test"),
					"generation": []byte("4"),
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
