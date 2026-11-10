# Развёртывание

Один образ api описан в deploy/images.yaml; Dockerfile app/Dockerfile, context '.'.
Сохранить структуру deploy/base/deployment.yaml, deploy/base/service.yaml и deploy/overlays/demo/.
Deployment и Service называются smoke-api. Namespace hack-sandbox-release создаёт оператор.
Readiness и liveness используют /healthz на именованном порту http (8080).
Штатные сборки, публикацию SHA-тегов и синхронизацию Argo выполняет Conductor.
Не менять вручную SHA-теги, не запускать kubectl/helm/argocd из кодовых заданий.
В этом smoke HTTPRoute не нужен, несмотря на общий шаблон инструкций S3. Доступ через port-forward.
Не добавлять Namespace, Secret, RBAC, ServiceAccount, NetworkPolicy и PVC в Application проекта:
ограничивающие ресурсы уже подготовлены оператором. Приложению не нужен доступ к API Kubernetes.
Не требуется deploy/secrets.yaml: никаких прикладных секретов нет.
Проверки в tasks.yaml: check_env {kind: compose, file: compose.yaml, service: dev};
checks [{name: contract, cmd: 'python -m unittest discover -s tests/contract -v'}];
e2e {name: e2e, cmd: 'python -m unittest discover -s tests/e2e -v'}.
