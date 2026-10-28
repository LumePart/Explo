# explo

Helm chart for [Explo](https://github.com/LumePart/Explo).

## Install

```bash
helm repo add explo https://lumepart.github.io/Explo
helm repo update
helm install explo explo/explo --namespace explo --create-namespace
```

## Example values

```yaml
ingress:
  enabled: true
  className: traefik
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
  hosts:
    - host: explo.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: explo-tls
      hosts: [explo.example.com]

persistence:
  config:
    storageClass: longhorn
  media:
    existingClaim: music-library
    subPath: explo

extraEnv:
  EXPLO_SYSTEM: jellyfin
  SYSTEM_URL: http://jellyfin.jellyfin.svc.cluster.local:8096
  API_KEY: your-api-key
  LIBRARY_NAME: Music
```


Config is stored under `/opt/explo/config` (`WEB_ENV_PATH`). Downloads go to `/data`.

See `values.yaml` for all knobs.
