FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/changelogger .

FROM gcr.io/distroless/static
COPY --from=builder /bin/changelogger /usr/local/bin/changelogger
ENTRYPOINT ["changelogger"]
