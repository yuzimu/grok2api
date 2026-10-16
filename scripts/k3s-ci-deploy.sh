#!/usr/bin/env bash
set -Eeuo pipefail
PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
export PATH
umask 077

# SSH 强制命令只允许更新固定部署的主容器，不接收任意命令或其他镜像。
if [[ $EUID != 0 || $# != 1 || ! $1 =~ ^deploy\ (registry\.cn-hongkong\.aliyuncs\.com/xbrd/grok2api@sha256:[a-f0-9]{64})$ ]]; then
  echo "Only immutable Grok2API K3s deployments are permitted" >&2
  exit 64
fi
image=${BASH_REMATCH[1]}
install -d -m 700 /opt/grok2api-ci/backups
exec 9>/opt/grok2api-ci/deploy.lock
flock -n 9 || { echo "Grok2API deployment already running" >&2; exit 75; }
kubectl -n xbrd get deployment grok2api -o yaml \
  > "/opt/grok2api-ci/backups/deployment-$(date -u +%Y%m%dT%H%M%SZ).yaml"
kubectl -n xbrd set image deployment/grok2api "grok2api=$image"
kubectl -n xbrd rollout status deployment/grok2api --timeout=300s
actual=$(kubectl -n xbrd get deployment grok2api -o jsonpath='{.spec.template.spec.containers[?(@.name=="grok2api")].image}')
[[ $actual == "$image" ]]
printf 'Deployed %s\n' "$image"
