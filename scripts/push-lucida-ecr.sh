#!/usr/bin/env bash
set -euo pipefail

REGION="${AWS_REGION:-ap-southeast-1}"
REPOSITORY="${ECR_REPOSITORY:-void-remover-lucida}"
TAG="${IMAGE_TAG:-latest}"
PLATFORM="${LAMBDA_PLATFORM:-linux/arm64}"

command -v aws >/dev/null 2>&1 || {
  echo "AWS CLI v2 is required." >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || {
  echo "Docker is required." >&2
  exit 1
}

ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
REGISTRY="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"
IMAGE_URI="${REGISTRY}/${REPOSITORY}:${TAG}"

if ! aws ecr describe-repositories \
  --region "${REGION}" \
  --repository-names "${REPOSITORY}" >/dev/null 2>&1; then
  aws ecr create-repository \
    --region "${REGION}" \
    --repository-name "${REPOSITORY}" \
    --image-scanning-configuration scanOnPush=true \
    --image-tag-mutability MUTABLE >/dev/null
fi

aws ecr put-lifecycle-policy \
  --region "${REGION}" \
  --repository-name "${REPOSITORY}" \
  --lifecycle-policy-text "file://lambda/lucida/ecr-lifecycle-policy.json" \
  >/dev/null

aws ecr get-login-password --region "${REGION}" |
  docker login --username AWS --password-stdin "${REGISTRY}"

docker buildx build \
  --platform "${PLATFORM}" \
  --provenance=false \
  --load \
  --tag "${IMAGE_URI}" \
  lambda/lucida

docker push "${IMAGE_URI}"

echo "Pushed ${IMAGE_URI}"
