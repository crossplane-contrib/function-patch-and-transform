# function-conditional-patch-and-transform

[![CI](https://github.com/stevendborrelli/function-conditional-patch-and-transform/actions/workflows/ci.yml/badge.svg)](https://github.com/stevendborrelli/function-conditional-patch-and-transform/actions/workflows/ci.yml) ![GitHub release (latest SemVer)](https://img.shields.io/github/release/crossplane-contrib/function-conditional-patch-and-transform)

This composition function is a fork of the upstream [function-patch-and-transform](https://github.com/crossplane-contrib/function-patch-and-transform)
that adds support for Conditional invocation of the function and the rendering
of individual resources.

## Installing this Function

The function can be installed as a Crossplane package, and runs in a [Composition Function](https://docs.crossplane.io/latest/concepts/composition-functions/). This feature requires a minium Crossplane version of 1.14.

```yaml
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-conditional-patch-and-transform
  annotations:
    render.crossplane.io/runtime: Development
spec:
  package: xpkg.upbound.io/borrelli-org/function-conditional-patch-and-transform:v0.4.0
```

## What this function does

This function enables conditional rendering of the entire function or select resources.

The language used for Conditionals is the [Common Expression Language (CEL)](https://github.com/google/cel-spec), which is widely used in the Kubernetes ecosystem.

### Conditionally Running the Function

Composition authors express a CEL condition, and if it returns `true`, patch-and-transforms defined in the `input` will be processed.

```yaml
 mode: Pipeline
  pipeline:
  - step: patch-and-transform
    functionRef:
      name: function-patch-and-transform
    input:
      apiVersion: conditional-pt.fn.crossplane.io/v1beta1
      kind: Resources
      condition: observed.composite.resource.spec.env == "prod" && observed.composite.resource.spec.render == true
      resources: [...all your resources...]
```

Using the following XR and RunFunctionRequest inputs (click to expand):
<details>

```yaml
apiVersion: nopexample.org/v1alpha1
kind: XNopResource
metadata:
  name: test-resource
spec:
  env: dev
  render: true
```

```json
{
   "desired": {
      "composite": {
         "resource": {
            "apiVersion": "nopexample.org/v1alpha1",
            "kind": "XNopResource",
            "metadata": {
               "name": "test-resource"
            },
            "spec": {
               "env": "dev",
               "render": true
            }
         }
      },
      "resources": {
         "test": {
            "resource": {
               "apiVersion": "example.org/v1",
               "kind": "CD",
               "metadata": {
                  "name": "cool-42",
                  "namespace": "default"
               }
            }
         }
      }
   },
   "observed": {
      "composite": {
         "resource": {
            "apiVersion": "nopexample.org/v1alpha1",
            "kind": "XNopResource",
            "metadata": {
               "name": "test-resource"
            },
            "spec": {
               "env": "dev",
               "render": true
            },
            "status": {
               "id": "123",
               "ready": false
            }
         }
      }
   }
}
```

</details>

You can use the  [CEL Playground](https://playcel.undistro.io/?content=H4sIAAAAAAAAA%2B1UPW%2FDIBT8K4g5SeW0U9Z27tCh6sDyYl5aVAwIsNUq8n%2BvMY4dGxx16dYNuON93D04Uw4e6IGemSKEMMrRCYuc0QOJR%2F1pqSujnfA4O%2B8hi07XtkyQHgQjXtE6oVWAGVXa4BdURuJO2%2Fe7pgBpPqBgdLO8%2BSkUj3fenrV5GZMkxAo9hB4y%2BXtcQYUxkEfnt1O5c26bBHYGy7WgqJoYk2OT1DTIojjaQPK2xkWuq%2B24ngqYNHWp3KGJrNQ3fCA5K%2BY%2B5JuYTHh8yjNuq0%2FmBpRay%2B3DPhdpZDoDJV60PUEt%2FdKphYCrevaLQVVG9dGhbf4H%2B28HO83lwdfJGF9QMShR7O%2FXkgH%2FDpwTSPerVxRdZ6qlm27ETadKMKn74Fjn1BH4lU5EKDJ8d7vxxdH2B6myt7YTBQAA) to test various queries.

Here are some example queries on the XR and RunFunctionRequest:

- `desired.composite.resource.spec.env == "dev"` evaluates to  `true`
- `desired.composite.resource.spec.render == true,` evaluates to `true`
- `desired.composite.resource.spec.render == false"` evaluates to `false`
- `observed.composite.resource.status.ready == true"` evaluates to `false`
- `size(desired.resources) == 0` evaluates to `false`
- `"test" in desired.resources`evaluates to `true`
- `"bad-resource" in desired.resources` evaluates to `false`

### Conditionally Rendering Managed Resources

In a similar manner, individual Managed Resources can also
be rendered conditionally, see the example at [examples/conditional-resources](examples/conditional-resources/). 

Each resource can have a `condition`.

```yaml
      resources:
         - name: blue-resource
           condition: observed.composite.resource.spec.deployment.blue == true
           base:
            apiVersion: nop.crossplane.io/v1alpha1
            kind: NopResource
            spec:
              forProvider:
```

If this condition is set in the Claim/XR, the resource will be rendered:

```yaml
apiVersion: nop.example.org/v1alpha1
kind: XNopConditional
metadata:
  name: test-resource
spec:
  env: dev
  render: true
  deployment:
    blue: true
    green: false

```

### Test this function locally using the Crossplane CLI

You can use the Crossplane CLI to run any function locally and see what composed
resources it would create. This only works with functions - not native P&T.

For example, using the files in the [examples](examples) directory:

```shell
cd examples/conditional-rendering
crossplane beta render xr.yaml composition.yaml functions.yaml
```

Produces the following output, showing what resources Crossplane would compose:

```yaml
---
apiVersion: nop.example.org/v1alpha1
kind: XNopResource
metadata:
  name: test-resource
---
apiVersion: nop.crossplane.io/v1alpha1
kind: NopResource
metadata:
  annotations:
    crossplane.io/composition-resource-name: test-resource
  generateName: test-resource-
  labels:
    crossplane.io/composite: test-resource
  ownerReferences:
  - apiVersion: nop.example.org/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: XNopResource
    name: test-resource
    uid: ""
spec:
  forProvider:
    conditionAfter:
    - conditionStatus: "True"
      conditionType: Ready
      time: 5s
    connectionDetails:
    - name: username
      value: fakeuser
    - name: password
      value: verysecurepassword
    - name: endpoint
      value: 127.0.0.1
    fields:
      arrayField:
      - stringField: array
      integerField: 42
      objectField:
        stringField: object
      stringField: string
```

See the [composition functions documentation][docs-functions] to learn how to
use `crossplane beta render`.

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