# Example multistep pipeline

Example Functions Pipeline with the 3 steps:


1. function-patch-and-transform
2. function-go-templating
3. function-patch-and-transform again

The more details mechanics of each step described below

1. function-patch-and-transform creates `bucket`
2. function-go-templating creates `bucketACL` and uses the data from the
   previous pipelines step to compose the resource spec
```
                region: {{ .desired.resources.bucket.resource.spec.forProvider.region }}
```
3. function-patch-and-transform is used again to patch the `bucketACL` with the
   data from XR spec. Notice that `base` is omitted and resource `name` is
matching the one that is set by the function-go-templating with `{{ setResourceNameAnnotation "bucketACL" }}`

To render `make render` target is available:

```
crossplane beta render xr.yaml composition.yaml functions.yaml -r
---
apiVersion: example.crossplane.io/v1
kind: XR
metadata:
  name: example-xr
status:
  conditions:
  - lastTransitionTime: "2024-01-01T00:00:00Z"
    message: 'Unready resources: bucket, bucketACL'
    reason: Creating
    status: "False"
    type: Ready
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
  - apiVersion: example.crossplane.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: XR
    name: example-xr
    uid: ""
spec:
  forProvider:
    region: eu-north-1
---
apiVersion: s3.aws.upbound.io/v1beta1
kind: BucketACL
metadata:
  annotations:
    crossplane.io/composition-resource-name: bucketACL
  generateName: example-xr-
  labels:
    crossplane.io/composite: example-xr
  ownerReferences:
  - apiVersion: example.crossplane.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: XR
    name: example-xr
    uid: ""
spec:
  forProvider:
    acl: private
    bucketSelector:
      matchControllerRef: true
    region: eu-north-1
```

Notice that `BucketACL` is patched as expected

```
spec:
  forProvider:
    acl: private # acl value that is set on 3rd pipeline step is in place
    bucketSelector:
      matchControllerRef: true
    region: eu-north-1 # region value that is set on 2nd pipeline step is in place
```
