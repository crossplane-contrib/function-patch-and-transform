package main

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"
	"github.com/crossplane/crossplane-runtime/pkg/test"
)

func TestRenderFromJSON(t *testing.T) {
	errInvalidChar := json.Unmarshal([]byte("olala"), &fake.Composed{})

	type args struct {
		o    resource.Object
		data []byte
	}
	type want struct {
		o   resource.Object
		err error
	}
	cases := map[string]struct {
		reason string
		args
		want
	}{
		"InvalidData": {
			reason: "We should return an error if the data can't be unmarshalled",
			args: args{
				o:    &fake.Composed{},
				data: []byte("olala"),
			},
			want: want{
				o:   &fake.Composed{},
				err: errors.Wrap(errInvalidChar, errUnmarshalJSON),
			},
		},
		"ExistingGVKChanged": {
			reason: "We should return an error if unmarshalling the base template changed the composed resource's group, version, or kind",
			args: args{
				o: composed.New(composed.FromReference(corev1.ObjectReference{
					APIVersion: "example.org/v1",
					Kind:       "Potato",
				})),
				data: []byte(`{"apiVersion": "example.org/v1", "kind": "Different"}`),
			},
			want: want{
				o: composed.New(composed.FromReference(corev1.ObjectReference{
					APIVersion: "example.org/v1",
					Kind:       "Different",
				})),
				err: errors.Errorf(errFmtKindChanged, "example.org/v1, Kind=Potato", "example.org/v1, Kind=Different"),
			},
		},
		"NewComposedResource": {
			reason: "A valid base template should apply successfully to a new (empty) composed resource",
			args: args{
				o:    composed.New(),
				data: []byte(`{"apiVersion": "example.org/v1", "kind": "Potato", "spec": {"cool": true}}`),
			},
			want: want{
				o: &composed.Unstructured{Unstructured: unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "example.org/v1",
						"kind":       "Potato",
						"spec": map[string]any{
							"cool": true,
						},
					},
				}},
			},
		},
		"ExistingComposedResource": {
			reason: "A valid base template should apply successfully to a new (empty) composed resource",
			args: args{
				o: composed.New(composed.FromReference(corev1.ObjectReference{
					APIVersion: "example.org/v1",
					Kind:       "Potato",
					Name:       "ola-superrandom",
				})),
				data: []byte(`{"apiVersion": "example.org/v1", "kind": "Potato", "spec": {"cool": true}}`),
			},
			want: want{
				o: &composed.Unstructured{Unstructured: unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "example.org/v1",
						"kind":       "Potato",
						"metadata": map[string]any{
							"name": "ola-superrandom",
						},
						"spec": map[string]any{
							"cool": true,
						},
					},
				}},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := RenderFromJSON(tc.args.o, tc.args.data)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nRenderFromJSON(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, tc.args.o); diff != "" {
				t.Errorf("\n%s\nRenderFromJSON(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
