# Conditional Rendering of Individual Resources

In this example, we show how each resource in a pipeline step can
be individually rendered based on a condition. We will have a blue and a
green deployment that the user can activate:

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

In our Composition both the `blue-resource` and the `green-resource` have a
`condition` that determines if they will be run.

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

## Running This Example Locally

This example can be rendered using `crossplane beta render`:

```shell
crossplane beta render xr.yaml composition.yaml functions.yaml
```

## Running this example in a Cluster

- Install Crossplane version 1.14 or newer. See <https://docs.crossplane.io/v1.14/software/install/>
- Install the nop provider in `kubectl apply -f provider.yaml`
- Install the XRD & Composition in `kubectl apply -f definition.yaml -f composition.yaml`
- Install the Function `kubectl apply -f functions.yaml`

Finally install the xr: `kubectl apply -f xr`
