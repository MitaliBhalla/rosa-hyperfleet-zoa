FROM registry.access.redhat.com/ubi9/go-toolset:1.26.5-1787080706 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG ZOA_VERSION=0.2.0
ARG GIT_COMMIT=unknown

USER 0
RUN mkdir -p /workspace && chown 1001:0 /workspace
USER 1001

WORKDIR /workspace
ENV GOCACHE=/workspace/.cache/go-build

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) && \
    VERSION_PKG="github.com/openshift-online/rosa-hyperfleet-zoa/internal/version" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -buildvcs=false \
    -ldflags="-w -s -X ${VERSION_PKG}.Version=${ZOA_VERSION} -X ${VERSION_PKG}.GitCommit=${GIT_COMMIT} -X ${VERSION_PKG}.BuildDate=${BUILD_DATE}" \
    -o /workspace/zoa-lambda ./cmd/zoa-lambda/

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8-1786987521

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

COPY --from=builder /workspace/zoa-lambda /usr/local/bin/zoa-lambda

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/zoa-lambda"]
