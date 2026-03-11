FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/schema-gen .

FROM alpine:3
COPY --from=builder /bin/schema-gen /usr/local/bin/schema-gen
ENTRYPOINT ["schema-gen"]
