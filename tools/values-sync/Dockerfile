FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/values-sync .

FROM alpine:3.24
RUN apk add --no-cache git
COPY --from=builder /bin/values-sync /usr/local/bin/values-sync
ENTRYPOINT ["values-sync"]
