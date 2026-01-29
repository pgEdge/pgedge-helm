variable "CHART_VERSION" {
  default = "0.0.4"
}

variable "BUILD_REVISION" {
  default = "1"
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

# Development build - tags as :dev (local only)
target "dev" {
  inherits = ["default"]
  tags = [
    "${IMAGE_NAME}:dev"
  ]
  output = ["type=docker"]
}

# Development build - push :dev to registry
target "dev-push" {
  inherits = ["default"]
  tags = [
    "${REGISTRY}/${IMAGE_NAME}:dev"
  ]
  output = ["type=registry"]
  platforms = [
    "linux/amd64",
    "linux/arm64",
  ]
}

# Push to registry - builds and pushes versioned tags
target "release" {
  inherits = ["default"]
  tags = [
    "${REGISTRY}/${IMAGE_NAME}:v${CHART_VERSION}-${BUILD_REVISION}",
    "${REGISTRY}/${IMAGE_NAME}:v${CHART_VERSION}",
  ]
  output = ["type=registry"]

  platforms = [
    "linux/amd64",
    "linux/arm64",
  ]
  attest = [
    "type=provenance,mode=min",
    "type=sbom",
  ]
}

