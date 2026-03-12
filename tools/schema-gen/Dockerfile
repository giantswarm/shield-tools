FROM golang:1.26-alpine AS builder

ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -o /bin/schema-gen .

FROM gcr.io/distroless/static
COPY --from=builder /bin/schema-gen /usr/local/bin/schema-gen
ENTRYPOINT ["schema-gen"]
