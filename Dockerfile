FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /grey ./cmd/grey

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /grey /usr/local/bin/grey
EXPOSE 9101
ENTRYPOINT ["grey"]
CMD ["-config", "/etc/grey/config.yaml"]
