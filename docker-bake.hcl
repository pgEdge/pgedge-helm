# Docker Bake configuration for pgedge-helm-utils
# Usage: docker buildx bake [target]

variable "CHART_VERSION" {
  default = "0.0.4"
}

variable "REGISTRY" {
  default = "ghcr.io/pgedge"
}

variable "IMAGE_NAME" {
  default = "pgedge-helm-utils"
}

# Common configuration for all targets
target "default" {
  context    = "."
  dockerfile = "docker/pgedge-helm-utils/Dockerfile"
}

# Development build - tags as :dev
target "dev" {
  inherits = ["default"]
  tags = [
    "${IMAGE_NAME}:dev"
  ]
  output = ["type=docker"]
}

# Versioned build - tags with chart version and latest
target "release" {
  inherits = ["default"]
  tags = [
    "${IMAGE_NAME}:${CHART_VERSION}",
    "${IMAGE_NAME}:latest"
  ]
  output = ["type=docker"]
}

# Push to registry - builds and pushes versioned tags
target "push" {
  inherits = ["release"]
  tags = [
    "${REGISTRY}/${IMAGE_NAME}:${CHART_VERSION}",
    "${REGISTRY}/${IMAGE_NAME}:latest"
  ]
  output = ["type=registry"]
}

# Load into kind - builds dev and outputs for kind load
target "kind" {
  inherits = ["dev"]
  output = ["type=docker"]
  # kind load will be done via makefile
}

# Load into minikube - builds dev and outputs for minikube load
target "minikube" {
  inherits = ["dev"]
  output = ["type=docker"]
  # minikube load will be done via makefile
}
