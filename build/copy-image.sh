#!/bin/zsh
set -ex
DOCKER_REGISTRY=$1
TARGET_REGISTRY=$2

# shellcheck disable=SC2034
for loop in controller gateway store timer trigger
	do
	  docker pull "${DOCKER_REGISTRY}"/vanus/controller:"${IMAGE_TAG}"
	  docker tag "${DOCKER_REGISTRY}"/vanus/controller:"${IMAGE_TAG}" "${TARGET_REGISTRY}"/vanus/controller:"${IMAGE_TAG}"
	  docker push "${TARGET_REGISTRY}"/vanus/controller:"${IMAGE_TAG}"
	done