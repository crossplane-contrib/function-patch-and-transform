# Conditional Execution of Patch-and-Transform

This is an experimental fork using [The Common Expression Language (CEL)](https://github.com/google/cel-spec) to conditionally running the patch and transform function.

For example,if you have an XR with the following manifest:

```yaml
apiVersion: nop.example.org/v1alpha1
kind: XNopResource
metadata:
  name: test-resource
spec:
  env: dev
  render: false
```

and set a Condition of `observed.composite.resource.spec.render == true`, the function
will not run.

## Running this example

- Install Crossplane version 1.14 or newer. See <https://docs.crossplane.io/v1.14/software/install/>
- Install the nop provider in `kubectl apply -f provider.yaml`
- Install the XRD & Composition in `kubectl apply -f definition.yaml -f composition.yaml`
- Install the Function `kubectl apply -f function.yaml`

Finally install the xr: `kubectl apply -f xr`

## Validate the Install

Validate the Composite is ready:

```shell
$ kubectl get composite 
NAME            SYNCED   READY   COMPOSITION                     AGE
test-resource   True     True    xnopresources.nop.example.org   2m9s
```

Validate the resource has been generated.

```shell
$ kubectl get nopresource
NAME                  READY   SYNCED   AGE
test-resource-mpw5s   True    True     27m
```

## Testing Conditions

The `RunFunctionRequest` passed to the function looks similar to the following manifest.
This can be used in the [CEL Playground](https://playcel.undistro.io) to test
various conditions.

Note that the function is passed an empty `desired{}` state as per design.

```yaml
---
observed:
  composite:
    resource:
      apiVersion: nop.example.org/v1alpha1
      kind: XNopResource
      metadata:
        name: test-resource
      spec:
        env: dev
        render: true
desired: {}
input:
  apiVersion: pt.fn.crossplane.io/v1beta1
  condition:
    expression: observed.composite.resource.spec.env == "dev" && observed.composite.resource.spec.render
      == true
  kind: Resources
  resources:
  - base:
      apiVersion: nop.crossplane.io/v1alpha1
      kind: NopResource
      spec:
        forProvider:
          conditionAfter:
          - conditionStatus: 'True'
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
    name: test-resource
```
