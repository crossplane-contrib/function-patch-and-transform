# function-patch-and-transform
[![CI](https://github.com/crossplane-contrib/function-patch-and-transform/actions/workflows/ci.yml/badge.svg)](https://github.com/crossplane-contrib/function-patch-and-transform/actions/workflows/ci.yml) ![GitHub release (latest SemVer)](https://img.shields.io/github/release/crossplane-contrib/function-patch-and-transform)

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
      name: function-patch-and-transform
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

There are a lot of good reasons to use a function to do P&T
composition. In fact, it's so compelling that the Crossplane maintainers are
considering deprecating native P&T. See Crossplane issue [#4746] for details.

### Mix and match P&T with other functions

With this function you can use P&T with other functions. For example you can
create a desired resource using the [Go Templating][fn-go-templating] function,
then patch the result using this function.

To include results from previous functions, simply provide a `resources` entry
for each and specify a `name` field that matches the name of the resource from
the previous function. Also, do not specify any value for the `base` field of
each resource.

It's not just patches either. You can use P&T to derive composite resource
connection details from a resource produced by another function, or use it to
determine whether a resource produced by another function is ready.

A straightforward example for multistep mix and match pipeline with
function-patch-and-transform and function-go-templating can be found
[here](./example/multistep)

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

### New Required fields

These fields are now required. This makes P&T configuration less ambiguous:

* `resources[i].name`
* `resources[i].connectionDetails[i].name`
* `resources[i].connectionDetails[i].type`
* `resources[i].patches[i].transforms[i].string.type`
* `resources[i].patches[i].transforms[i].math.type`

### `mergeOptions` replaced by `toFieldPath`

Also, the `resources[i].patches[i].policy.mergeOptions` field is no longer
supported. This functionality has been replaced by the
`resources[i].patches[i].policy.toFieldPath` field. The table below outlines
previous behavior that was possible with `mergeOptions` and how to achieve it
with the new `toFieldPath` field:

| # | `mergeOptions` | `appendSlice`| `keepMapValues` | `toFieldPath` |
| - | -------------  | ------------ | --------------- | -------------------- |
| 1 | `nil`  | N/A | N/A | `nil` which defaults to `Replace` |
| 2 | `non-nil` | `nil` or `false` | `true` | `MergeObjects` |
| 3 | `non-nil` | `true` | `nil` or `false` | `ForceMergeObjectsAppendArrays` |
| 4 | `non-nil` | `nil` or `false` |  `nil` or `false` |  `ForceMergeObjects`   |
| 5 | `non-nil` | `true` | `true` | `MergeObjectsAppendArrays` |

As an example, a previous configuration using the no longer supported `mergeOptions`:

```yaml
policy:
  mergeOptions:
    appendSlice: true
    keepMapValues: true
```

Should be replaced with:

```yaml
policy:
  toFieldPath: MergeObjectsAppendArrays
```

Starting with Crossplane v1.16.0, the `convert` command in the [Crossplane
CLI][cli-convert] will automatically convert `mergeOptions` to `toFieldPath` for
you.

## XR Connection details

This function handles composite resource connection details differently
depending on if the XR is Crossplane `v1` or `v2` style.

* `v1`: Connection details are returned from the function pipeline and Crossplane
  creates a connection secret for the XR/claim.
* `v2`: This function automatically composes a `Secret` containing the connection
  details and includes it along with the XR's other composed resources.

A full [connection details guide][docs-connection-details] can be found in the
Crossplane documentation.

### Setting name/namespace

For v2 XRs, you can control the name and namespace of this connection secret in
a few ways, in order of precedence:

**XR reference:**

If you've manually included a `spec.writeConnectionSecretToRef` in your XR's
schema, this function will use that reference. This can be useful for maintaining
consistency with existing XR configurations.

**Function `input`:**

A `writeConnectionSecretToRef` specified in the function `input` that has at
least one of name or namespace set:

```yaml
input:
  apiVersion: pt.fn.crossplane.io/v1beta1
  kind: Resources
  writeConnectionSecretToRef:
    name: my-app-credentials
    namespace: production
```

**Default auto generated**

If none of the above options are provided, the function generates a name based
on the XR's name (`{xr-name}-connection`) and uses the XR's namespace if it has
one. Note this will not work for cluster scoped XR's because there is no
namespace to store the `Secret` in. You must specify a connection secret
namespace for cluster scoped XRs if you want connection secret functionality.

### Patching secret name/namespace

For v2 XRs, you can also use patches to dynamically construct the secret name or
namespace from XR fields. This is useful when you want the secret name to
include environment-specific information or other metadata:

```yaml
writeConnectionSecretToRef:
  patches:
  - type: CombineFromComposite
    toFieldPath: name
    combine:
      variables:
      - fromFieldPath: metadata.name
      - fromFieldPath: spec.parameters.environment
      strategy: string
      string:
        fmt: "%s-%s-credentials"
```

Patches support the same `FromCompositeFieldPath` and `CombineFromComposite`
types available for resource patches (and only those patch types), and can
target either `name` or `namespace` fields.

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
[docs-functions]: https://docs.crossplane.io/latest/concepts/compositions/
[docs-pandt]: https://docs.crossplane.io/latest/guides/function-patch-and-transform/
[docs-connection-details]: https://docs.crossplane.io/latest/guides/connection-details-composition/
[fn-go-templating]: https://github.com/crossplane-contrib/function-go-templating
[#4617]: https://github.com/crossplane/crossplane/issues/4617
[#4746]: https://github.com/crossplane/crossplane/issues/4746
[go]: https://go.dev
[docker]: https://www.docker.com
[cli]: https://docs.crossplane.io/latest/cli
[cli-convert]: https://docs.crossplane.io/latest/cli/command-reference/#beta-convert
