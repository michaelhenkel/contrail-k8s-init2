```
docker build -t contrail-k8s-init-container .
```
to trigger a new pull:
```
docker build -t contrail-k8s-init-container --build-arg source=`date +%s` .
```
