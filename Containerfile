FROM registry.access.redhat.com/ubi9/go-toolset:1.26.5-1786351949 AS builder
USER root

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG ZOA_VERSION=0.2.0

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown") && \
    BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) && \
    VERSION_PKG="github.com/openshift-online/rosa-hyperfleet-zoa/internal/version" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -buildvcs=false \
    -ldflags="-w -s -X ${VERSION_PKG}.Version=${ZOA_VERSION} -X ${VERSION_PKG}.GitCommit=${GIT_COMMIT} -X ${VERSION_PKG}.BuildDate=${BUILD_DATE}" \
    -o /app/zoa-lambda ./cmd/zoa-lambda/

# Lambda Web Adapter (enables response streaming for API mode)
FROM public.ecr.aws/awsguru/aws-lambda-adapter:1.0.1 AS lwa

# Minimal Lambda runtime — Go static binary needs no OS dependencies
FROM public.ecr.aws/lambda/provided:al2023-x86_64

ARG VERSION=0.0.1
ARG RELEASE=1

LABEL name="zoa-lambda" \
      vendor="Red Hat, Inc." \
      version="${VERSION}" \
      release="${RELEASE}" \
      summary="ZOA Lambda function" \
      description="Minimal Lambda image for ZOA API and Worker" \
      io.k8s.display-name="zoa-lambda" \
      io.k8s.description="ZOA Lambda function for API and Worker execution" \
      com.redhat.component="zoa-lambda-container" \
      distribution-scope="public" \
      url="https://github.com/openshift-online/rosa-hyperfleet-zoa"

COPY --from=builder /app/zoa-lambda /usr/local/bin/zoa-lambda
COPY --from=lwa /lambda-adapter /opt/extensions/lambda-adapter

ENTRYPOINT ["/usr/local/bin/zoa-lambda"]
