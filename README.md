# function-patch-and-transform

A [Crossplane] Composition Function that implements P&T-style Composition.

## What is this?

This [Composition Function][function-design] does everything Crossplane's built-in Patch &
Transform Composition does. Instead of specifying `spec.resources` in your
Composition, you can use this Function.

Note that this is a beta-style Function. It won't work with Crossplane v1.13 or
earlier - it targets the [implementation of Functions][function-pr] coming with
Crossplane v1.14 in late October.

Take [this example](docs-composition) from https://docs.crossplane.io. Using
this Function, it would look like this:

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: dynamo-with-bucket
spec:
  compositeTypeRef:
    apiVersion: database.example.com/v1alpha1
    kind: NoSQL
  pipeline:
  - step: patch-and-transform
    functionRef:
      name: function-patch-and-transform
    input:
      apiVersion: pt.fn.crossplane.io/v1beta1
      kind: Resources
      resources:
      - name: s3Bucket
        base:
          apiVersion: s3.aws.upbound.io/v1beta1
          kind: Bucket
          metadata:
            name: crossplane-quickstart-bucket
          spec:
            forProvider:
              region: us-east-2
        patches:
        - type: FromCompositeFieldPath
          fromFieldPath: "location"
          toFieldPath: "spec.forProvider.region"
          transforms:
          - type: map
            map: 
              EU: "eu-north-1"
              US: "us-east-2"
      - name: dynamoDB
        base:
          apiVersion: dynamodb.aws.upbound.io/v1beta1
          kind: Table
          metadata:
            name: crossplane-quickstart-database
          spec:
            forProvider:
              region: "us-east-2"
              writeCapacity: 1
              readCapacity: 1
              attribute:
              - name: S3ID
                type: S
              hashKey: S3ID
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

Notice that it looks pretty much identical to the example from the Crossplane
documentation. The key difference is that everything that used to be under
`spec.resources` is now nested a little deeper. Specifically, it's under
`spec.pipeline[0].input.resources` (i.e. in the Function's input).

## Okay, but why?

I think there are a _lot_ of good reasons to implement P&T Composition as a
Function. In fact, I would go so far as to propose that once Functions are a GA
feature we deprecate support for 'native' P&T (i.e. `spec.resources`). We can't
remove it - that would be a breaking change - but we can freeze its API and
suggest folks use the P&T Function instead.

### Run P&T anywhere in your pipeline

Native P&T can only run before the Composition Function pipeline. In the [draft
beta implementation of Functions][function-pr] Crossplane does all the patching
and transforming first, then sends the results through the Function pipeline.

This is handy, but what if you wanted to run another Function (like rendering
some Go templates) first, then pass the result of that Function to be patched
and transformed? With this Function you can do that:

```yaml
 apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: dynamo-with-bucket
spec:
  compositeTypeRef:
    apiVersion: database.example.com/v1alpha1
    kind: NoSQL
  # This pipeliene renders some Go templates, then passes them to P&T
  pipeline:
  - step: render-go-templates
    functionRef:
      name: function-go-templates
    input: {} # Omitted for brevity :)
  - step: patch-and-transform
    functionRef:
      name: function-patch-and-transform
    input:
      apiVersion: pt.fn.crossplane.io/v1beta1
      kind: Resources
      resources:
        # Notice that my-cool-bucket doesn't have a base template. As long as
        # the render-go-templates step above rendered a composed resource with
        # this name, this Function will patch it.
      - name: my-cool-bucket
        patches:
        - type: FromCompositeFieldPath
          fromFieldPath: "location"
          toFieldPath: "spec.forProvider.region"
          transforms:
          - type: map
            map: 
              EU: "eu-north-1"
              US: "us-east-2"
```

It's not just patches either - you can use P&T to derive XR connection details
from a resource produced by another Function too, or use it to determine whether
a resource produced by another Function is ready

### Decouple P&T development from Crossplane core

When P&T development happens in a Function, it's not coupled to the Crossplane
release cycle. The Function developers could cut releases more frequently to add
new features to P&T.

Plus, because it's just a Function, it becomes easier to fork. You could fork
this Function, add a new kind of transform and try it out for a few weeks in
your development environment before sending a PR upstream. Or, if your new
feature is controversial, it's now a lot less work to maintain your own fork
long term.

### Makes P&T code more portable

A lot of building a better developer experience around Composition comes down to
shifting left - letting you run and test your Compositions when you're
developing them. Historically this has been tough. You need to spin up a `kind`
cluster, install Crossplane, install providers, etc. 

```bash
$ xp composition render xr.yaml composition.yaml
```

You could imagine a CLI tool like the above helping a lot. The problem with
building tools like this in the past has been that they need to share
Crossplane's Composition logic. We could make Composition a library, but then
you'd need to make sure that the version of `xp` on your laptop used the same
Composition library as your control planes, or you might see different results
than you expected in production.

When all Composition logic is encapsulated in Functions - i.e. versioned OCI
containers with a standard RPC - building a tool like this becomes much easier.
Just tell the CLI what Function versions you're using in production and it can
pull them down and use them to render your Composition.

### Makes Crossplane's Composition implementation simpler

If we can make the _native_ P&T implementation and Functions mutually exclusive,
Crossplane's Composition implementation is dramatically less complex. This means 
it's easier to maintain and much less likely to be buggy.

Moving P&T inside a Function makes this possible - you can still use 'both' P&T
and Functions, you'd just do it by... using Functions.

Eventually, if enough P&T users switch to this Function we may be able to remove
native support for P&T altogether.

## Differences from the native implementation

This Function has a few small, intentional breaking changes compared to the
native implementation. Making the below fields required makes P&T configuration
a lot more explicit and less ambiguous.

* `resources[i].name` is now a required field.
* `resources[i].connectionDetails[i].name` is now a required field
* `resources[i].connectionDetails[i].type` is now a required field
* `resources[i].patches[i].policy.mergeOptions` will have its fields renamed
  (once reimplemented) to address [#2581].

## Known issues

The initial implementation has the following limitations:

* It's not actually packaged as an installable Function yet. :)
* It can't report that composed resources are ready, because this isn't yet
  supported by `RunFunctionRequest`.
* `EnvironmentConfig` and its associated patches aren't supported yet. This is
  just because Crossplane doesn't yet send the `EnvironmentConfig` along with
  the `RunFunctionRequest`. Once we do, these should be easy to (re)implement.
* `patches[i].policy.mergeOptions` is not supported yet. Part of the
  implementation relied on `ApplyOptions`, which don't exist in Functions. I
  need to follow-up to see how to re-add support.
* All of the code in `sdk.go` needs to move to crossplane/function-sdk-go.

## Developing

This Function doesn't use the typical Crossplane build submodule and Makefile,
since we'd like Functions to have a less heavyweight developer experience.
It mostly relies on regular old Go tools:

```shell
# Run code generation - see input/generate.go
$ go generate ./...

# Run tests
$ go test -cover ./...
?       github.com/negz/function-patch-and-transform/input/v1beta1      [no test files]
ok      github.com/negz/function-patch-and-transform    0.021s  coverage: 76.1% of statements

# Lint the code
$ docker run --rm -v $(pwd):/app -v ~/.cache/golangci-lint/v1.54.2:/root/.cache -w /app golangci/golangci-lint:v1.54.2 golangci-lint run

# Build a Docker image - see Dockerfile
$ docker build .
```

[Crossplane]: https://crossplane.io
[function-design]: https://github.com/crossplane/crossplane/blob/3996f20/design/design-doc-composition-functions.md
[function-pr]: https://github.com/crossplane/crossplane/pull/4500
[docs-composition]: https://docs.crossplane.io/v1.13/getting-started/provider-aws-part-2/#create-a-deployment-template
[#2581]: https://github.com/crossplane/crossplane/issues/2581
