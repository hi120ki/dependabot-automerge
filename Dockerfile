ARG GO_VERSION=1.22

FROM golang:${GO_VERSION} as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM gcr.io/distroless/base

WORKDIR /app

COPY --chown=nonroot:nonroot --from=builder /app/main .
COPY --chown=nonroot:nonroot --from=builder /app/config.yaml .

USER nonroot

CMD ["./main"]
