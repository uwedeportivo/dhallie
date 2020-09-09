templating engine as part of solution for the assignment in https://github.com/sourcegraph/sourcegraph/issues/13335

```shell script
dhallie -c ds-dhall/src/base/components.yaml \
        -t ds-dhall/src/apply/apply-deployment-image.dhall-template \
        -o apply-deployment-image.dhall
```
