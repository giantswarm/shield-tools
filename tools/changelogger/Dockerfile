FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/changelogger .

FROM alpine:3
COPY --from=builder /bin/changelogger /usr/local/bin/changelogger
ENTRYPOINT ["changelogger"]
