FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/schema-gen .

FROM gcr.io/distroless/static
COPY --from=builder /bin/schema-gen /usr/local/bin/schema-gen
ENTRYPOINT ["schema-gen"]
