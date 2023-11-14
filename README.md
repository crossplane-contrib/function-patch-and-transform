# function-conditional-patch-and-transform
[![CI](https://github.com/stevendborrelli/function-conditional-patch-and-transform/actions/workflows/ci.yml/badge.svg)](https://github.com/stevendborrelli/function-conditional-patch-and-transform/actions/workflows/ci.yml) ![GitHub release (latest SemVer)](https://img.shields.io/github/release/crossplane-contrib/function-conditional-patch-and-transform)

This composition function is a fork of the upstream [function-patch-and-transform](https://github.com/crossplane-contrib/function-patch-and-transform)
that adds support for Conditional invocation of the function and the rendering
of individual resources.

This [composition function][docs-functions] does everything Crossplane's
built-in [patch & transform][docs-pandt] (P&T) composition does. Instead of
specifying `spec.resources` in your Composition, you can use this function.

Using this function, P&T looks like this:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: example
spec:
  # Omitted for brevity.
  mode: Pipeline
  pipeline:
  - step: patch-and-transform
    functionRef:
      name: function-conditional-patch-and-transform
    input:
      apiVersion: pt.fn.crossplane.io/v1beta1
      kind: Resources
      resources:
      - name: bucket
        base:
          apiVersion: s3.aws.upbound.io/v1beta1
          kind: Bucket
          spec:
            forProvider:
              region: us-east-2
        patches:
        - type: FromCompositeFieldPath
          fromFieldPath: "spec.location"
          toFieldPath: "spec.forProvider.region"
          transforms:
          - type: map
            map: 
              EU: "eu-north-1"
              US: "us-east-2"
```

Notice that it looks very similar to native P&T. The difference is that
everything is under `spec.pipeline[0].input.resources`, not `spec.resources`.
This is the Function's input.

## Okay, but why?

There are a lot of good reasons to use a function to use a function to do P&T
composition. In fact, it's so compelling that the Crossplane maintainers are
considering deprecating native P&T. See Crossplane issue [#4746] for details.

### Mix and match P&T with other functions

With this function you can use P&T with other functions. For example you can
create a desired resource using the [Go Templating][fn-go-templating] function,
then patch the result using this function.

It's not just patches either. You can use P&T to derive composite resource
connection details from a resource produced by another function, or use it to
determine whether a resource produced by another function is ready

### Decouple P&T development from Crossplane core

When P&T development happens in a function, it's not coupled to the Crossplane
release cycle. The maintainers of this function can cut releases more frequently
to add new features to P&T.

It also becomes easier to fork. You could fork this function, add a new kind of
transform and try it out for a few weeks before sending a PR upstream. Or, if
your new feature is controversial, it's now a lot less work to maintain your own
fork long term.

### Test P&T locally using the Crossplane CLI

You can use the Crossplane CLI to run any function locally and see what composed
resources it would create. This only works with functions - not native P&T.

For example, using the files in the [example](example) directory:

```shell
$ crossplane beta render xr.yaml composition.yaml functions.yaml
```
Produces the following output, showing what resources Crossplane would compose:

```yaml
---
apiVersion: example.crossplane.io/v1
kind: XR
metadata:
  name: example-xr
---
apiVersion: s3.aws.upbound.io/v1beta1
kind: Bucket
metadata:
  annotations:
    crossplane.io/composition-resource-name: bucket
  generateName: example-xr-
  labels:
    crossplane.io/composite: example-xr
  ownerReferences:
    # Omitted for brevity
spec:
  forProvider:
    region: us-east-2
```

See the [composition functions documentation][docs-functions] to learn how to
use `crossplane beta render`.

## Differences from the native implementation

This function has a few small, intentional breaking changes compared to the
native implementation.

These fields are now required. This makes P&T configuration less ambiguous:

* `resources[i].name`
* `resources[i].connectionDetails[i].name`
* `resources[i].connectionDetails[i].type`
* `resources[i].patches[i].transforms[i].string.type`
* `resources[i].patches[i].transforms[i].math.type`

Also, the `resources[i].patches[i].policy.mergeOptions` field is no longer
supported.

Composition functions use Kubernetes server-side apply to intelligently merge
arrays and objects. This requires merge configuration to be specified at the
composed resource schema level (i.e. in CRDs) per [#4617].

## Developing this function

This function uses [Go][go], [Docker][docker], and the [Crossplane CLI][cli] to
build functions.

```shell
# Run code generation - see input/generate.go
$ go generate ./...

# Run tests - see fn_test.go
$ go test ./...

# Build the function's runtime image - see Dockerfile
$ docker build . --tag=runtime

# Build a function package - see package/crossplane.yaml
$ crossplane xpkg build -f package --embed-runtime-image=runtime
```

[Crossplane]: https://crossplane.io
[docs-composition]: https://docs.crossplane.io/v1.14/getting-started/provider-aws-part-2/#create-a-deployment-template
[docs-functions]: https://docs.crossplane.io/v1.14/concepts/composition-functions/
[docs-pandt]: https://docs.crossplane.io/v1.14/concepts/patch-and-transform/
[fn-go-templating]: https://github.com/stevendborrelli/function-go-templating
[#4617]: https://github.com/crossplane/crossplane/issues/4617
[#4746]: https://github.com/crossplane/crossplane/issues/4746
[go]: https://go.dev
[docker]: https://www.docker.com
[cli]: https://docs.crossplane.io/latest/cli